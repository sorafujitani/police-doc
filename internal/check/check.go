package check

import (
	"fmt"
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
		if example.Code == "invalid-shell" {
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
	sources := func() []spec.Evidence {
		if len(command.Sources) > 0 {
			return command.Sources
		}
		return snapshot.Sources
	}
scan:
	for i := 1; i < len(example.Words); i++ {
		if len(command.Sources) == 0 && len(command.Flags) == 0 {
			break // A placeholder provides no evidence about its flags or arguments.
		}
		token := example.Words[i]
		if !token.Static {
			add("info", "uncheckable", "dynamic-argument", "Only the static prefix was checked; expansions and substitutions were not executed.", sources())
			break
		}
		word := token.Value
		if word == "--" {
			break // The remaining tail may be forwarded to another program.
		}
		if strings.HasPrefix(word, "-") && word != "-" {
			name, _, attached := strings.Cut(word, "=")
			flag := command.Flag(name)
			if flag == nil {
				add("warning", "needs-review", "unverified-flag", fmt.Sprintf("%s is absent from the collected help; this does not prove it is invalid or removed.", name), sources())
				break
			}
			switch flag.Value {
			case "required":
				if !attached {
					if i+1 == len(example.Words) {
						add("error", "confirmed", "missing-flag-value", name+" requires a value.", sources())
					} else if !example.Words[i+1].Static {
						add("info", "uncheckable", "dynamic-argument", "The flag value and following arguments are dynamic; they were not expanded or checked.", sources())
						break scan
					} else {
						i++ // Even a dash-prefixed token can be a flag's value.
					}
				}
			case "optional", "unknown":
				if !attached {
					break scan // The next token may be a value, not a subcommand or flag.
				}
			}
			continue
		}
		if child := command.Child(word); child != nil {
			command = child
			continue
		}
		// Help does not establish positional bounds or forwarding semantics.
		// Stop rather than checking a script's arguments against its launcher.
		add("info", "uncheckable", "unchecked-arguments", "Positional arguments and the remaining tail were not checked.", sources())
		break
	}
	code, message := "incomplete-spec", "Help is incomplete; hidden entries, argument bounds and lifecycle changes are not checked."
	if len(command.Sources) == 0 && len(command.Flags) == 0 {
		result.Coverage = "uncheckable"
		code, message = "unavailable-spec", "The command is known, but its help has not been collected."
	}
	add("info", "uncheckable", code, message, sources())
	return result
}
