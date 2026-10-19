package spec

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// synopsisForms accepts a small, explicit usage grammar, never prose. If any
// alternative is ambiguous, none of its positional limits become authoritative.
func synopsisForms(output, name string, path []string, command *Command) []Form {
	var forms []Form
	for _, words := range helpSynopses(output) {
		if len(words) < len(path)+1 || strings.TrimSuffix(strings.TrimSuffix(filepath.Base(words[0]), "[EXE]"), ".exe") != name {
			continue
		}
		matches := true
		for i, part := range path {
			matches = matches && slices.Contains(strings.Split(strings.Trim(words[i+1], "[]():"), "|"), part)
		}
		if !matches {
			continue
		}
		tail := words[len(path)+1:]
		if helpStartsWithSubcommand(tail) && len(command.Commands) > 0 {
			continue // Dispatch has its own command catalog and child grammar.
		}
		p := synopsisParser{input: strings.Join(tail, " "), command: command}
		alternatives, ok := p.group(0)
		if !ok || p.input != "" || len(forms)+len(alternatives) > 64 {
			return nil
		}
		forms = append(forms, alternatives...)
	}
	if len(command.Commands) > 0 {
		onlyZero := true
		for _, f := range forms {
			onlyZero = onlyZero && f.MaxArgs != nil && *f.MaxArgs == 0
		}
		if onlyZero {
			return nil
		} // A command catalog may stand in for a missing command placeholder.
	}
	return forms
}

type synopsisParser struct {
	input   string
	command *Command
}

func exactForm(count int) Form { return Form{MinArgs: count, MaxArgs: &count} }

func combineForms(left, right []Form) ([]Form, bool) {
	if len(left)*len(right) > 64 {
		return nil, false
	}
	var result []Form
	for _, a := range left {
		for _, b := range right {
			f := Form{MinArgs: a.MinArgs + b.MinArgs, Required: slices.Clone(a.Required)}
			for _, name := range b.Required {
				if !slices.Contains(f.Required, name) {
					f.Required = append(f.Required, name)
				}
			}
			if a.MaxArgs != nil && b.MaxArgs != nil {
				n := *a.MaxArgs + *b.MaxArgs
				f.MaxArgs = &n
			}
			result = append(result, f)
		}
	}
	return compactForms(result), true
}

func compactForms(forms []Form) []Form {
	var result []Form
	for _, f := range forms {
		covered := false
		for _, previous := range result {
			sameMax := previous.MaxArgs == nil && f.MaxArgs == nil || previous.MaxArgs != nil && f.MaxArgs != nil && *previous.MaxArgs == *f.MaxArgs
			if sameMax && previous.MinArgs == f.MinArgs && (len(previous.Required) == 0 || slices.Equal(previous.Required, f.Required)) {
				covered = true
				break
			}
		}
		if !covered {
			if len(f.Required) == 0 {
				result = slices.DeleteFunc(result, func(previous Form) bool {
					return previous.MinArgs == f.MinArgs && (previous.MaxArgs == nil && f.MaxArgs == nil || previous.MaxArgs != nil && f.MaxArgs != nil && *previous.MaxArgs == *f.MaxArgs)
				})
			}
			result = append(result, f)
		}
	}
	return result
}

// Bracketed sequences keep their cardinality: [SRC DST] allows zero or two,
// not one. Repeated multi-word groups are left unresolved rather than widened.
func (p *synopsisParser) group(depth int) ([]Form, bool) {
	if depth > 16 {
		return nil, false
	}
	forms := []Form{exactForm(0)}
	for {
		p.input = strings.TrimSpace(p.input)
		if p.input == "" || p.input[0] == ']' {
			return forms, true
		}
		var atom []Form
		if p.input[0] == '[' {
			p.input = p.input[1:]
			var ok bool
			atom, ok = p.group(depth + 1)
			if !ok || p.input == "" || p.input[0] != ']' {
				return nil, false
			}
			p.input = p.input[1:]
			for _, branch := range atom {
				if len(branch.Required) > 0 && (branch.MaxArgs == nil || *branch.MaxArgs > 0) {
					return nil, false
				}
			}
			atom = compactForms(append([]Form{exactForm(0)}, atom...))
		} else {
			word := p.word()
			if word == "" {
				return nil, false
			}
			if word == "--" {
				return nil, false
			} // May introduce a forwarded command.
			if strings.HasPrefix(word, "-") {
				flag := p.command.Flag(word)
				if flag == nil || (flag.Value != "none" && flag.Value != "required") {
					return nil, false
				}
				f := exactForm(0)
				f.Required = []string{flag.Name}
				atom = []Form{f}
				if flag.Value == "required" {
					for range max(1, flag.ValueCount) {
						if !synopsisOperand(p.word(), true) {
							return nil, false
						}
					}
				}
			} else if synopsisOptions(word) {
				if depth == 0 || len(forms) != 1 || forms[0].MinArgs != 0 || forms[0].MaxArgs == nil || *forms[0].MaxArgs != 0 {
					return nil, false
				}
				atom = []Form{exactForm(0)}
			} else if synopsisOperand(word, depth > 0) {
				atom = []Form{exactForm(1)}
			} else {
				return nil, false
			}
		}
		p.input = strings.TrimSpace(p.input)
		if strings.HasPrefix(p.input, "...") {
			p.input = p.input[3:]
			for i := range atom {
				if atom[i].MaxArgs == nil || *atom[i].MaxArgs > 1 {
					return nil, false
				}
				if *atom[i].MaxArgs == 1 {
					atom[i].MaxArgs = nil
				}
			}
		}
		var ok bool
		forms, ok = combineForms(forms, atom)
		if !ok {
			return nil, false
		}
	}
}

func (p *synopsisParser) word() string {
	p.input = strings.TrimSpace(p.input)
	if p.input == "" {
		return ""
	}
	end := 0
	if p.input[0] == '<' {
		if i := strings.IndexByte(p.input, '>'); i >= 0 {
			end = i + 1
		}
	} else {
		for end < len(p.input) && !strings.ContainsRune(" \t[](){}|", rune(p.input[end])) && !strings.HasPrefix(p.input[end:], "...") {
			end++
		}
	}
	word := p.input[:end]
	p.input = p.input[end:]
	return word
}

func synopsisOptions(word string) bool {
	switch strings.ToLower(word) {
	case "option", "options", "flags", "opts":
		return true
	}
	return false
}

func synopsisOperand(word string, bracketed bool) bool {
	if word == "" {
		return false
	}
	explicit := strings.HasPrefix(word, "<") && strings.HasSuffix(word, ">")
	word = strings.TrimSuffix(strings.TrimPrefix(word, "<"), ">")
	switch strings.ToLower(word) {
	case "command", "commands", "subcommand", "subcommands", "args", "arguments", "expression", "options", "flags":
		return false
	}
	for _, r := range word {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_.-", r) {
			return false
		}
	}
	return explicit || bracketed || word == strings.ToUpper(word)
}
