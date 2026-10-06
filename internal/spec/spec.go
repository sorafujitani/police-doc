package spec

import (
	"fmt"
	"slices"
	"strings"
)

// Snapshot contains only observations from an installed CLI's version and help.
// Help cannot establish complete grammar or lifecycle metadata.
type Snapshot struct {
	Tool    string     `json:"tool"`
	Version string     `json:"version"`
	OS      string     `json:"os"`
	Sources []Evidence `json:"sources,omitempty"`
	Root    Command    `json:"root"`
}

type Evidence struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	Detail    string `json:"detail,omitempty"`
}

type Command struct {
	// UsageName is the program name printed in help, which may differ from argv[0].
	UsageName string `json:"usage_name,omitempty"`
	// SubcommandFirst means every matching usage starts with a command placeholder
	// (possibly after options), rather than an ordinary positional argument.
	SubcommandFirst       bool       `json:"subcommand_first,omitempty"`
	FlagsAfterPositionals bool       `json:"flags_after_positionals,omitempty"`
	ShortFlagClusters     bool       `json:"short_flag_clusters,omitempty"`
	AliasesComplete       bool       `json:"aliases_complete,omitempty"`
	Name                  string     `json:"name,omitempty"`
	Aliases               []string   `json:"aliases,omitempty"`
	Commands              []Command  `json:"commands,omitempty"`
	Flags                 []Flag     `json:"flags,omitempty"`
	Sources               []Evidence `json:"sources,omitempty"`
}

type Flag struct {
	Name    string   `json:"name"`
	Value   string   `json:"value"` // none, required, optional-attached, optional, or unknown.
	Choices []string `json:"choices,omitempty"`
}

func (s *Snapshot) Validate() error {
	if !validName(s.Tool) || strings.TrimSpace(s.Version) == "" || s.Version == "latest" {
		return fmt.Errorf("tool and a concrete version are required")
	}
	if s.Root.Name != "" {
		return fmt.Errorf("root must not have a name")
	}
	if err := validateEvidence(s.Sources); err != nil {
		return err
	}
	return validateCommand(&s.Root, s.Tool)
}

func validateCommand(c *Command, path string) error {
	if c.UsageName != "" && !validName(c.UsageName) {
		return fmt.Errorf("%s: invalid help program name", path)
	}
	aliases := make(map[string]bool)
	for _, alias := range c.Aliases {
		if !validName(alias) || aliases[alias] {
			return fmt.Errorf("%s: invalid or repeated alias %q", path, alias)
		}
		aliases[alias] = true
	}
	if err := validateEvidence(c.Sources); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	flags := make(map[string]bool)
	for _, flag := range c.Flags {
		if flag.Value != "none" && flag.Value != "required" && flag.Value != "optional-attached" && flag.Value != "optional" && flag.Value != "unknown" {
			return fmt.Errorf("%s: %s has invalid value mode %q", path, flag.Name, flag.Value)
		}
		if !validFlag(flag.Name) || flags[flag.Name] {
			return fmt.Errorf("%s: invalid or conflicting flag %q", path, flag.Name)
		}
		choices := make(map[string]bool)
		for _, choice := range flag.Choices {
			if strings.TrimSpace(choice) == "" || strings.ContainsAny(choice, "\r\n\x00\x1b") || choices[choice] {
				return fmt.Errorf("%s: invalid or repeated choice for %s", path, flag.Name)
			}
			choices[choice] = true
		}
		flags[flag.Name] = true
	}
	names := make(map[string]bool)
	for i := range c.Commands {
		child := &c.Commands[i]
		if !validName(child.Name) || names[child.Name] {
			return fmt.Errorf("%s: invalid or conflicting command %q", path, child.Name)
		}
		names[child.Name] = true
		if err := validateCommand(child, path+" "+child.Name); err != nil {
			return err
		}
	}
	return nil
}

func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, " \t\r\n/\\=") && !strings.HasPrefix(name, "-")
}

func validFlag(name string) bool {
	if strings.HasPrefix(name, "--") {
		return len(name) > 2 && !strings.HasPrefix(name[2:], "-") && !strings.ContainsAny(name[2:], " \t\r\n=/\\")
	}
	// Single-dash long flags (such as Go's -race) are also supported.
	return strings.HasPrefix(name, "-") && len(name) > 1 && !strings.ContainsAny(name[1:], " \t\r\n=/\\")
}

func validateEvidence(evidence []Evidence) error {
	for _, item := range evidence {
		if item.Kind != "help" || strings.TrimSpace(item.Reference) == "" {
			return fmt.Errorf("help evidence requires a reference")
		}
	}
	return nil
}

func (c *Command) Child(name string) *Command {
	for i := range c.Commands {
		if c.Commands[i].Name == name {
			return &c.Commands[i]
		}
	}
	if !c.AliasesComplete {
		return nil // Do not choose an alias before sibling conflicts can be checked.
	}
	var alias *Command
	for i := range c.Commands {
		if slices.Contains(c.Commands[i].Aliases, name) {
			if alias != nil {
				return nil // Conflicting aliases are not a usable command mapping.
			}
			alias = &c.Commands[i]
		}
	}
	return alias
}

func (c *Command) Flag(name string) *Flag {
	for i := range c.Flags {
		if c.Flags[i].Name == name {
			return &c.Flags[i]
		}
	}
	return nil
}
