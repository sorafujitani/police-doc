package check

import (
	"slices"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func contractFixture() *spec.Snapshot {
	maxArgs := 2
	evidence := []spec.Evidence{{Kind: "reference", Reference: "https://example.invalid/acme/1.0/manual"}}
	return &spec.Snapshot{Tool: "acme", Version: "1.0", Sources: evidence, Root: spec.Command{
		Closed: true, SubcommandFirst: true, AliasesComplete: true, Sources: evidence,
		Commands: []spec.Command{{Name: "copy", Aliases: []string{"cp"}, Closed: true,
			Sources: evidence, FlagsAfterPositionals: true, ShortFlagClusters: true,
			Forms: []spec.Form{{MinArgs: 2, MaxArgs: &maxArgs}},
			Flags: []spec.Flag{
				{Name: "--output", Aliases: []string{"-o"}, Value: "required"},
				{Name: "--force", Aliases: []string{"-f"}, Value: "none"},
				{Name: "--mode", Value: "required", Choices: []string{"fast", "safe"}},
			},
		}},
	}}
}

func TestReferenceArgumentGrammar(t *testing.T) {
	for _, tc := range []struct{ command, code string }{
		{"acme copy from to", ""},
		{"acme cp -foresult from to", ""},
		{"acme copy from to --mode fast", ""},
		{"acme copy -- --not-an-option target", ""},
		{"acme copy from", "invalid-arguments"},
		{"acme copy from to extra", "invalid-arguments"},
		{"acme copy from to --output", "missing-flag-value"},
		{"acme copy from to --force=yes", "unexpected-flag-value"},
		{"acme copy from to --mode typo", "unverified-flag-value"},
		{"acme copy from to --typo", "unknown-flag"},
		{"acme typo from to", "unknown-command"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			result := resultFor(t, tc.command, contractFixture())
			if tc.code == "" {
				if result.Coverage != "checked" || len(result.Diagnostics) != 0 {
					t.Fatalf("not completely checked: %+v", result)
				}
			} else if !slices.ContainsFunc(result.Diagnostics, func(d Diagnostic) bool {
				return d.Code == tc.code && d.Severity == "error" && d.Status == "confirmed"
			}) {
				t.Fatalf("expected %s: %+v", tc.code, result)
			}
		})
	}
}

func TestReferenceFormsAndFlagOrdering(t *testing.T) {
	snapshot := contractFixture()
	command := &snapshot.Root.Commands[0]
	zero, two := 0, 2
	command.Flags = append(command.Flags, spec.Flag{Name: "--list", Value: "none"})
	command.Forms = []spec.Form{
		{MinArgs: 0, MaxArgs: &zero, Required: []string{"--list"}},
		{MinArgs: 2, MaxArgs: &two, Forbidden: []string{"--list"}},
	}
	for _, text := range []string{"acme copy --list", "acme copy a b"} {
		result := resultFor(t, text, snapshot)
		if result.Coverage != "checked" || len(result.Diagnostics) != 0 {
			t.Fatalf("valid alternative failed: %+v", result)
		}
	}
	for _, text := range []string{"acme copy", "acme copy --list a"} {
		if result := resultFor(t, text, snapshot); !slices.ContainsFunc(result.Diagnostics, func(d Diagnostic) bool { return d.Code == "invalid-arguments" }) {
			t.Fatalf("invalid alternative accepted: %+v", result)
		}
	}
	command.FlagsAfterPositionals = false
	if result := resultFor(t, "acme copy a b --force", snapshot); !slices.ContainsFunc(result.Diagnostics, func(d Diagnostic) bool { return d.Code == "flag-after-operand" }) {
		t.Fatalf("invalid flag order accepted: %+v", result)
	}
	command.StopFlagsAfterOperand = true
	if result := resultFor(t, "acme copy a --", snapshot); result.Coverage != "checked" || len(result.Diagnostics) != 0 {
		t.Fatalf("double dash was discarded after option parsing stopped: %+v", result)
	}
	if result := resultFor(t, "acme copy a --literal", snapshot); result.Coverage != "checked" || len(result.Diagnostics) != 0 {
		t.Fatalf("literal dash operand was checked as an option: %+v", result)
	}
}

func TestAncestorRequirementsAreChecked(t *testing.T) {
	snapshot := contractFixture()
	snapshot.Root.Flags = []spec.Flag{{Name: "--token", Value: "required"}}
	snapshot.Root.Forms = []spec.Form{{Required: []string{"--token"}}}
	if result := resultFor(t, "acme --token value copy from to", snapshot); result.Coverage != "checked" || len(result.Diagnostics) != 0 {
		t.Fatalf("valid parent requirements were lost: %+v", result)
	}
	if result := resultFor(t, "acme copy from to", snapshot); !slices.ContainsFunc(result.Diagnostics, func(d Diagnostic) bool { return d.Code == "invalid-arguments" }) {
		t.Fatalf("missing parent requirement was ignored: %+v", result)
	}
}

func TestScalarValuesDoNotStopLaterChecks(t *testing.T) {
	result := resultFor(t, `acme copy --output "$OUT" from to --mode fast`, contractFixture())
	if result.Coverage != "checked" || len(result.Diagnostics) != 0 {
		t.Fatalf("a scalar string argument prevented checking later flags: %+v", result)
	}
	result = resultFor(t, `acme copy --output "$OUT" from to --typo`, contractFixture())
	if !slices.ContainsFunc(result.Diagnostics, func(d Diagnostic) bool { return d.Code == "unknown-flag" }) {
		t.Fatalf("later typo was skipped: %+v", result)
	}
	for _, text := range []string{`acme copy --mode "$MODE" a b`, `acme copy --output $ARGS a b`, `acme copy --output "$@" a b`, `acme copy --output "${v:-"$@"}" a b`, `acme copy --output "${!prefix@}" a b`, `acme copy --output "$(unknown --bad)" a b`, `ENV=$(unknown --bad) acme copy a b`, `acme copy --output $"translated" a b`} {
		if result := resultFor(t, text, contractFixture()); result.Coverage == "checked" {
			t.Fatalf("unknown choices or word counts were declared checked: %+v", result)
		}
	}
}
