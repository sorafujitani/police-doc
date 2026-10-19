package check

import "slices"

// Requirement is missing evidence or input, not an approval or suppression.
// Every incomplete result retains actionable requirements, including diagnostics
// from forwarded children. Supplying them requires another actual check.
type Requirement struct {
	Kind   string `json:"kind"`
	Code   string `json:"code"`
	Action string `json:"action"`
}

func requirements(result Result) []Requirement {
	if result.Coverage == "checked" {
		return nil
	}
	var needs []Requirement
	for _, d := range result.Diagnostics {
		if d.Status != "uncheckable" && d.Status != "needs-review" {
			continue
		}
		if d.Code == "incomplete-spec" && len(result.Diagnostics) > 1 {
			continue
		}
		r := Requirement{Code: d.Code}
		switch d.Code {
		case "unsupported-cli", "help-unavailable", "missing-reference", "builtin-help-unavailable", "shell-builtin":
			r.Kind = "target-specification"
			r.Action = "Select the intended CLI or shell version and OS, then supply its reference with --spec. A missing executable is not evidence that its arguments are valid."
		case "help-incomplete", "unavailable-spec", "unverified-command", "unverified-flag", "unverified-flag-value", "unverified-arguments", "incomplete-spec", "unchecked-arguments", "unchecked-positionals", "unknown-flag-arity", "unverified-flag-order":
			r.Kind = "argument-grammar"
			r.Action = "Export the available rules with --export-spec, add the missing command, flag, value, ordering or positional rules from a versioned source, then rescan with --spec. Export alone does not resolve this gap."
		case "dynamic-argument", "dynamic-command":
			r.Kind = "concrete-input"
			r.Action = "Provide concrete command/argument values, or an example whose quoted operands have a known shape after --. Unbound expansions cannot be proven valid for every possible value."
		case "unchecked-substitution":
			r.Kind = "substitution-contract"
			r.Action = "Provide a separately checkable example for the nested command and a contract for its output shape. The substitution is not evaluated to discover either."
		case "forwarded-arguments", "forwarding-limit":
			r.Kind = "forwarding-contract"
			r.Action = "Declare the argv forwarding boundary and child target rules in the reference; replace unbounded or cyclic forwarding with separately checkable examples."
		case "unsupported-environment", "shell-binding":
			r.Kind = "shell-context"
			r.Action = "Identify the intended executable independently of PATH changes, aliases or functions, and provide a standalone example for that target. Environment changes are not executed."
		case "unsupported-wrapper":
			r.Kind = "wrapper-contract"
			r.Action = "Provide the resolved executable and its argv as a standalone example, with the selected version's reference. Wrappers and package installations are not executed."
		case "argument-fragment":
			r.Kind = "owning-command"
			r.Action = "Identify the CLI or configuration format owning these options. Use a full command example or the correct non-shell fence; options alone are not executable names."
		case "invalid-shell", "unsupported-shell":
			r.Kind = "document-context"
			r.Action = "Distinguish shell input, terminal output, interactive keystrokes and syntax notation. Correct invalid shell, supply a concrete template, or use the appropriate non-shell fence."
		default:
			r.Kind = "unresolved-rule"
			r.Action = "Resolve the reported missing rule or collection failure and rescan. This diagnostic has not been waived or counted as successful validation."
		}
		if !slices.Contains(needs, r) {
			needs = append(needs, r)
		}
	}
	if len(needs) == 0 {
		needs = append(needs, Requirement{Kind: "argument-grammar", Code: "incomplete-spec", Action: "Supply the missing argument rules and rescan; no complete verdict is available."})
	}
	return needs
}
