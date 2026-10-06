package spec

import (
	"context"
	"slices"
	"strings"
)

func helpAliases(output, program string, path []string) []string {
	if len(path) == 0 {
		return nil // Root aliases name executables, not child commands.
	}
	var aliases []string
	reading, body := false, false
	for line := range strings.SplitSeq(output, "\n") {
		trimmed := strings.TrimSpace(line)
		heading := strings.ToLower(strings.TrimSuffix(trimmed, ":"))
		if heading == "aliases" || heading == "alias" {
			reading, body = true, false
			continue
		}
		if trimmed == "" {
			if body {
				reading = false
			}
			continue
		}
		if strings.HasSuffix(trimmed, ":") || (line == trimmed && helpUpperHeading.MatchString(trimmed)) {
			reading = false
		}
		if !reading {
			continue
		}
		body = true
		words := strings.Fields(trimmed)
		if len(words) > 0 && words[0] == program {
			if len(words) != len(path)+1 || !slices.Equal(words[1:len(path)], path[:len(path)-1]) {
				continue // An invocation in another branch is not this command's alias.
			}
			words = words[len(path):] // e.g. "acme pr ls" identifies alias "ls".
		} else {
			words = nil
			valid := true
			for _, part := range strings.Split(trimmed, ",") {
				name := strings.TrimSpace(part)
				if name == "" {
					continue
				}
				if !helpCommandName.MatchString(name) {
					valid = false // A comma in prose does not make an alias list.
					break
				}
				words = append(words, name)
			}
			if !valid {
				continue
			}
		}
		for _, name := range words {
			if helpCommandName.MatchString(name) && name != path[len(path)-1] && !slices.Contains(aliases, name) {
				aliases = append(aliases, name)
			}
		}
	}
	return aliases
}

// CollectAliases only requests help for advertised siblings, never for the unknown
// word from the document. Its separate finite budget cannot starve normal checks.
func (c *HelpCollector) CollectAliases(ctx context.Context, path []string) error {
	parent := &c.Snapshot.Root
	for _, part := range path {
		parent = parent.Child(part)
		if parent == nil {
			return nil
		}
	}
	for i := range parent.Commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.aliasCalls >= c.options.MaxCommands {
			break
		}
		childPath := append(slices.Clone(path), parent.Commands[i].Name)
		// Failed sibling help is not evidence against this example. The original
		// unverified-command warning remains if no advertised alias is found.
		_ = c.collect(ctx, childPath, &c.aliasCalls)
	}
	complete := true
	for _, child := range parent.Commands {
		complete = complete && len(child.Sources) > 0
	}
	if parent.AliasesComplete != complete {
		parent.AliasesComplete = complete
		c.Revision++
	}
	return ctx.Err()
}
