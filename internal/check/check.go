package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

func Example(example extract.Example, snapshot *spec.Snapshot) Result {
	result := Result{Location: example.Location, Command: example.Text, CLI: example.CLI(), Coverage: "uncheckable", Diagnostics: []Diagnostic{}}
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
		add("info", "uncheckable", "unsupported-cli", fmt.Sprintf("No specification is available for %q.", example.CLI()), nil)
		return result
	}
	result.CLI, result.Version, result.OS, result.Shell = snapshot.Tool, snapshot.Version, snapshot.OS, snapshot.Shell
	result.Coverage = "partial"
	command := &snapshot.Root
	positionals, endOptions, complete := false, false, !example.HasSubstitution
	if example.HasSubstitution {
		add("info", "uncheckable", "unchecked-substitution", "Commands inside argument, assignment, or redirection substitutions were not checked; no substitution was executed.", nil)
	}
	argumentCount := 0
	seen := make(map[string]bool)
	var path []string
	evidence := command.Sources
	if len(evidence) == 0 {
		evidence = snapshot.Sources
	}
	result.Evidence = slices.Clone(evidence)
	sources := func() []spec.Evidence { return evidence }
scan:
	for i := 1; i < len(example.Words); i++ {
		if len(command.Sources) == 0 && len(command.Flags) == 0 {
			break
		}
		token := example.Words[i]
		if !token.Static {
			if token.Scalar && len(command.Forms) > 0 && (command.LiteralArguments || endOptions || (positionals && command.StopFlagsAfterOperand)) {
				argumentCount++ // A quoted scalar is one operand; its contents are not evaluated.
				continue
			}
			add("info", "uncheckable", "dynamic-argument", "The dynamic argument may change command grammar; the remaining arguments were not checked and expansions were not executed.", sources())
			complete = false
			break
		}
		word := token.Value
		if word == "--" && !command.LiteralArguments && !endOptions && !(positionals && command.StopFlagsAfterOperand) {
			if len(command.Forms) > 0 {
				endOptions = true
				continue
			}
			if i+1 < len(example.Words) {
				add("info", "uncheckable", "forwarded-arguments", "Arguments after -- were not checked.", sources())
			}
			complete = false
			break
		}
		isFlag := !command.LiteralArguments && !endOptions && !(positionals && command.StopFlagsAfterOperand) && strings.HasPrefix(word, "-") && word != "-"
		if isFlag {
			if positionals && len(command.Forms) > 0 && !command.FlagsAfterPositionals {
				if command.FlagOrderUnknown {
					add("warning", "uncheckable", "unverified-flag-order", "The synopsis does not establish how flags after operands are parsed; the remaining arguments were not checked.", sources())
					complete = false
					break
				}
				add("error", "confirmed", "flag-after-operand", "The reference grammar requires options before positional arguments.", sources())
			}
			for _, argument := range flagWords(command, word) {
				name, value, attached := strings.Cut(argument, "=")
				flag := command.Flag(name)
				if flag == nil {
					if command.Closed {
						add("error", "confirmed", "unknown-flag", fmt.Sprintf("%s is not allowed by the closed reference specification.", name), sources())
					} else {
						add("warning", "needs-review", "unverified-flag", fmt.Sprintf("%s is absent from the specification; this does not prove it is invalid or removed.", name), sources())
					}
					complete = false
					break scan
				}
				seen[flag.Name] = true
				if flag.Forward {
					if attached && flag.Terminator != "" && value == flag.Terminator {
						add("error", "confirmed", "missing-flag-value", name+" requires a command.", sources())
						continue
					}
					start, end := i+1, i+1
					for end < len(example.Words) && (flag.Terminator == "" || !example.Words[end].Static || example.Words[end].Value != flag.Terminator) {
						if flag.Terminator != "" && !example.Words[end].Static {
							add("info", "uncheckable", "dynamic-forwarding-boundary", "A dynamic word may be the forwarding terminator; the remaining arguments were not checked.", sources())
							complete = false
							break scan
						}
						end++
					}
					words := slices.Clone(example.Words[start:end])
					if attached {
						words = append([]extract.Word{{Value: value, Static: true}}, words...)
					}
					if len(words) == 0 || (words[0].Static && words[0].Value == "") {
						add("error", "confirmed", "missing-flag-value", name+" requires a command.", sources())
						if len(words) > 1 {
							add("info", "uncheckable", "forwarded-arguments", "Arguments without a forwarded executable were not checked.", sources())
							complete = false
						}
					} else {
						result.Forwarded = append(result.Forwarded, extract.Example{Location: example.Location, Text: example.Text, Words: words})
					}
					i = end - 1
					if end < len(example.Words) {
						i++ // The terminator belongs to the parent CLI, not the forwarded command.
					}
					continue
				}
				var values []extract.Word
				if attached {
					values = append(values, extract.Word{Value: value, Static: true})
					if flag.Value == "none" {
						if command.Closed {
							add("error", "confirmed", "unexpected-flag-value", name+" does not take a value.", sources())
						} else {
							add("info", "uncheckable", "unverified-flag-value", name+" has no documented attached-value rule.", sources())
							complete = false
						}
					}
				}
				switch flag.Value {
				case "required":
					count := max(1, flag.ValueCount)
					if attached && count > 1 {
						add("warning", "uncheckable", "unknown-flag-arity", fmt.Sprintf("%s documents %d separate values, but attached-value grouping is unknown; the remaining arguments were not checked.", name, count), sources())
						complete = false
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
						}
						if !example.Words[i+1].Static && !example.Words[i+1].Scalar {
							add("info", "uncheckable", "dynamic-argument", "The flag value may expand to multiple words; it was not evaluated.", sources())
							complete = false
							break scan
						}
						i++
						values = append(values, example.Words[i])
					}
				case "optional-separated":
					if !attached && i+1 < len(example.Words) && example.Words[i+1].Static && !strings.HasPrefix(example.Words[i+1].Value, "-") {
						i++
						values = append(values, example.Words[i])
					}
				case "optional", "unknown":
					severity := "info"
					if !attached && i+1 < len(example.Words) {
						severity = "warning"
					}
					add(severity, "uncheckable", "unknown-flag-arity", name+" has an unspecified value or attachment rule; that rule was not checked.", sources())
					complete = false
					if !attached {
						break scan
					}
				}
				for _, value := range values {
					if len(flag.Choices) == 0 {
						continue
					}
					if !value.Static {
						add("info", "uncheckable", "dynamic-choice", name+" has a dynamic value whose allowed choices cannot be checked.", sources())
						complete = false
					} else if !slices.Contains(flag.Choices, value.Value) {
						severity, status := "warning", "needs-review"
						if command.Closed {
							severity, status = "error", "confirmed"
						} else {
							complete = false
						}
						add(severity, status, "unverified-flag-value", fmt.Sprintf("%q is not among the documented choices for %s: %s.", value.Value, name, strings.Join(flag.Choices, ", ")), sources())
					}
				}
			}
			continue
		}
		if !endOptions && !positionals {
			if child := command.Child(word); child != nil {
				if complete && !command.DerivedForms && len(command.Forms) > 0 && !command.FormAccepts(argumentCount, seen) {
					add("error", "confirmed", "invalid-arguments", "The parent command does not match its argument form: "+command.FormDescription()+".", sources())
				}
				path = append(path, child.Name)
				command, seen = child, make(map[string]bool)
				if len(child.Sources) > 0 {
					evidence = child.Sources
					for _, source := range evidence {
						if !slices.Contains(result.Evidence, source) {
							result.Evidence = append(result.Evidence, source)
						}
					}
				}
				continue
			}
			if command.SubcommandFirst && len(command.Commands) > 0 {
				parent := strings.Join(append([]string{example.CLI()}, path...), " ")
				if command.Closed {
					add("error", "confirmed", "unknown-command", fmt.Sprintf("%q is not an allowed subcommand of %q.", word, parent), sources())
				} else {
					result.Alias = &AliasRequest{Path: slices.Clone(path), Name: word}
					add("warning", "needs-review", "unverified-command", fmt.Sprintf("%q could not be resolved as a listed subcommand or a unique documented alias of %q; this does not prove it is invalid or removed.", word, parent), sources())
				}
				complete = false
				break
			}
		}
		if len(command.Forms) > 0 {
			argumentCount++
			positionals = true
			continue
		}
		if command.FlagsAfterPositionals {
			if !positionals {
				add("info", "uncheckable", "unchecked-positionals", "Positional values and bounds were not checked; documented trailing flags are still checked.", sources())
			}
			positionals, complete = true, false
			continue
		}
		severity := "info"
		for _, remaining := range example.Words[i+1:] {
			if !remaining.Static || strings.HasPrefix(remaining.Value, "-") {
				severity = "warning"
				break
			}
		}
		add(severity, "uncheckable", "unchecked-arguments", "Positional arguments and the remaining tail were not checked; the specification does not establish their flag or forwarding rules.", sources())
		complete = false
		break
	}
	if len(command.Sources) == 0 && len(command.Flags) == 0 {
		result.Coverage, result.HelpPath = "uncheckable", path
		add("info", "uncheckable", "unavailable-spec", "The command is known, but its help has not been collected.", sources())
		return result
	}
	if complete && len(command.Forms) > 0 {
		if !command.FormAccepts(argumentCount, seen) {
			if command.DerivedForms {
				add("warning", "needs-review", "unverified-arguments", "Arguments do not match the collected synopsis; omitted modes or argument rules need a reference: "+command.FormDescription()+".", sources())
				return result
			}
			add("error", "confirmed", "invalid-arguments", "The command does not match a documented argument form: "+command.FormDescription()+".", sources())
		}
		result.Coverage = "checked"
		if len(result.Forwarded) > 0 {
			result.Coverage, result.OwnChecked = "partial", true
		}
	} else {
		add("info", "uncheckable", "incomplete-spec", "The specification does not establish every argument rule; this is not full static validation.", sources())
	}
	return result
}
