package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

func Example(example extract.Example, snapshot *spec.Snapshot) Result {
	result := Result{Location: example.Location, Command: example.Text, Coverage: "uncheckable", Diagnostics: []Diagnostic{}}
	add := func(severity, status, code, message string, evidence []spec.Evidence) {
		if evidence == nil {
			evidence = []spec.Evidence{}
		}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: severity, Status: status, Code: code, Message: message, Evidence: evidence})
	}
	if example.Reason != "" {
		severity := "info"
		if example.Code == "invalid-shell" || example.Code == "unsupported-environment" {
			severity = "warning"
		}
		add(severity, "uncheckable", example.Code, example.Reason, nil)
		return result
	}
	if len(example.Words) == 0 || !example.Words[0].Static {
		add("info", "uncheckable", "dynamic-command", "The executable is dynamic; no command was executed.", nil)
		return result
	}
	if snapshot == nil {
		add("info", "uncheckable", "unsupported-cli", fmt.Sprintf("No collected help is available for %q.", example.CLI()), nil)
		return result
	}
	result.CLI, result.Version, result.OS = snapshot.Tool, snapshot.Version, snapshot.OS
	result.Coverage = "partial"
	command := &snapshot.Root
	positionals := false
	var path []string
	evidence := command.Sources
	if len(evidence) == 0 {
		evidence = snapshot.Sources
	}
	sources := func() []spec.Evidence { return evidence }
scan:
	for i := 1; i < len(example.Words); i++ {
		if len(command.Sources) == 0 && len(command.Flags) == 0 {
			break // A placeholder provides no evidence about its flags or arguments.
		}
		token := example.Words[i]
		if !token.Static {
			add("info", "uncheckable", "dynamic-argument", "The dynamic argument and remaining tail were not checked; expansions and substitutions were not executed.", sources())
			break
		}
		word := token.Value
		if word == "--" {
			if i+1 < len(example.Words) {
				add("info", "uncheckable", "forwarded-arguments", "Arguments after -- were not checked.", sources())
			}
			break // The remaining tail may be forwarded to another program.
		}
		if strings.HasPrefix(word, "-") && word != "-" {
			for _, argument := range flagWords(command, word) {
				name, value, attached := strings.Cut(argument, "=")
				flag := command.Flag(name)
				if flag == nil {
					add("warning", "needs-review", "unverified-flag", fmt.Sprintf("%s is absent from the collected help; this does not prove it is invalid or removed.", name), sources())
					break scan
				}
				var values []string
				if attached {
					values = append(values, value)
				}
				switch flag.Value {
				case "required":
					count := max(1, flag.ValueCount)
					if attached && count > 1 {
						add("warning", "uncheckable", "unknown-flag-arity", fmt.Sprintf("%s documents %d separate values, but attached-value grouping is unknown; the remaining arguments were not checked.", name, count), sources())
						break scan
					}
					for len(values) < count {
						if i+1 == len(example.Words) {
							message := name + " requires a value."
							if count > 1 {
								message = fmt.Sprintf("%s requires %d values; only %d were provided.", name, count, len(values))
							}
							add("error", "confirmed", "missing-flag-value", message, sources())
							break scan
						} else if !example.Words[i+1].Static {
							add("info", "uncheckable", "dynamic-argument", "The flag value and following arguments are dynamic; they were not expanded or checked.", sources())
							break scan
						}
						i++ // Even a dash-prefixed token can be a flag's value.
						values = append(values, example.Words[i].Value)
					}
				case "optional", "unknown":
					if !attached {
						if i+1 < len(example.Words) {
							add("warning", "uncheckable", "unknown-flag-arity", name+" has an ambiguous value rule; the following arguments were not checked.", sources())
						}
						break scan
					}
				}
				for _, value := range values {
					if len(flag.Choices) > 0 && !slices.Contains(flag.Choices, value) {
						add("warning", "needs-review", "unverified-flag-value", fmt.Sprintf("%q is not among the documented choices for %s: %s.", value, name, strings.Join(flag.Choices, ", ")), sources())
					}
				}
			}
			continue
		}
		if child := command.Child(word); !positionals && child != nil {
			path = append(path, child.Name)
			command = child
			if len(child.Sources) > 0 {
				evidence = child.Sources
			}
			continue
		}
		if !positionals && command.SubcommandFirst && len(command.Commands) > 0 {
			result.Alias = &AliasRequest{Path: slices.Clone(path), Name: word}
			parent := strings.Join(append([]string{example.CLI()}, path...), " ")
			add("warning", "needs-review", "unverified-command", fmt.Sprintf("%q could not be resolved as a listed subcommand or a unique documented alias of %q; this does not prove it is invalid or removed.", word, parent), sources())
			break
		}
		if command.FlagsAfterPositionals {
			if !positionals {
				add("info", "uncheckable", "unchecked-positionals", "Positional values and bounds were not checked; documented trailing flags are still checked.", sources())
			}
			positionals = true
			continue
		}
		// Do not apply a launcher's flags to a forwarded script/command. Make
		// skipped flag-like words visible rather than silently reporting success.
		severity := "info"
		for _, remaining := range example.Words[i+1:] {
			if !remaining.Static || strings.HasPrefix(remaining.Value, "-") {
				severity = "warning"
				break
			}
		}
		add(severity, "uncheckable", "unchecked-arguments", "Positional arguments and the remaining tail were not checked; help does not establish their flag or forwarding rules.", sources())
		break
	}
	code, message := "incomplete-spec", "Help is incomplete; hidden entries, argument bounds and lifecycle changes are not checked."
	if len(command.Sources) == 0 && len(command.Flags) == 0 {
		result.Coverage = "uncheckable"
		result.HelpPath = path
		code, message = "unavailable-spec", "The command is known, but its help has not been collected."
	}
	add("info", "uncheckable", code, message, sources())
	return result
}
