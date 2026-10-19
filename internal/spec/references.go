package spec

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// References is an explicit, portable source of CLI specifications. Loading a
// reference never executes a binary, downloads data, or evaluates source text.
type References struct {
	SchemaVersion int        `json:"schema_version"`
	Tools         []Snapshot `json:"tools"`
}

func LoadReferences(paths []string) (map[string]*Snapshot, error) {
	result := make(map[string]*Snapshot)
	for _, path := range paths {
		paths := []string{path}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			paths, err = filepath.Glob(filepath.Join(path, "*.json"))
			if err != nil {
				return nil, err
			}
			if len(paths) == 0 {
				return nil, fmt.Errorf("%s: no reference files", path)
			}
		}
		for _, path := range paths {
			refs, err := readReferences(path)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			for i := range refs.Tools {
				snapshot := &refs.Tools[i]
				if result[snapshot.Key()] != nil {
					return nil, fmt.Errorf("multiple references for %q (shell=%q); select one target version", snapshot.Tool, snapshot.Shell)
				}
				result[snapshot.Key()] = snapshot
			}
		}
	}
	return result, nil
}

func readReferences(path string) (*References, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return nil, fmt.Errorf("reference must be a regular file of at most 8 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, (8<<20)+1))
	decoder.DisallowUnknownFields()
	var refs References
	if err := decoder.Decode(&refs); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("expected a single reference document")
	}
	if refs.SchemaVersion != 1 || len(refs.Tools) == 0 {
		return nil, fmt.Errorf("reference schema_version must be 1 and tools must not be empty")
	}
	for i := range refs.Tools {
		snapshot := &refs.Tools[i]
		if len(snapshot.Sources) == 0 {
			return nil, fmt.Errorf("%q: reference specifications require provenance", snapshot.Tool)
		}
		if err := prepareReference(&snapshot.Root, snapshot.Sources, 0); err != nil {
			return nil, fmt.Errorf("%q: %w", snapshot.Tool, err)
		}
		if err := snapshot.Validate(); err != nil {
			return nil, err
		}
	}
	return &refs, nil
}

func prepareReference(command *Command, parent []Evidence, depth int) error {
	if depth > 64 {
		return fmt.Errorf("reference command nesting exceeds 64 levels")
	}
	if len(command.Sources) == 0 {
		command.Sources = parent
	}
	// A reference lists its aliases explicitly; never probe document words.
	command.AliasesComplete = true
	names := make(map[string]bool)
	for i := range command.Commands {
		child := &command.Commands[i]
		for _, name := range append([]string{child.Name}, child.Aliases...) {
			if names[name] {
				return fmt.Errorf("conflicting command or alias %q", name)
			}
			names[name] = true
		}
		if err := prepareReference(child, command.Sources, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// FormAccepts checks the declared grammar, not external state such as whether a
// file, package, repository, or network endpoint exists.
func (c *Command) FormAccepts(count int, flags map[string]bool) bool {
	for _, form := range c.Forms {
		if count < form.MinArgs || (form.MaxArgs != nil && count > *form.MaxArgs) {
			continue
		}
		matches := true
		for _, name := range form.Required {
			matches = matches && flags[c.Flag(name).Name]
		}
		for _, name := range form.Forbidden {
			matches = matches && !flags[c.Flag(name).Name]
		}
		if matches {
			return true
		}
	}
	return false
}

func (c *Command) FormDescription() string {
	var forms []string
	for _, form := range c.Forms {
		bounds := fmt.Sprintf("at least %d positional argument(s)", form.MinArgs)
		if form.MaxArgs != nil {
			bounds = fmt.Sprintf("%d..%d positional argument(s)", form.MinArgs, *form.MaxArgs)
		}
		if len(form.Required) > 0 {
			bounds += "; requires " + strings.Join(form.Required, ", ")
		}
		if len(form.Forbidden) > 0 {
			bounds += "; excludes " + strings.Join(form.Forbidden, ", ")
		}
		forms = append(forms, bounds)
	}
	return strings.Join(forms, " or ")
}
