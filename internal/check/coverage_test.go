package check

import (
	"slices"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestUncertainRulesNeverBecomeChecked(t *testing.T) {
	for _, tc := range []struct{ value, argument string }{
		{"unknown", "--maybe=fast"},
		{"optional", "--maybe=fast"},
		{"optional", "--maybe fast"},
		{"none", "--maybe=fast"},
		{"required", "--maybe=unlisted"},
	} {
		snapshot := contractFixture()
		command := &snapshot.Root.Commands[0]
		command.Closed = false
		command.Flags = append(command.Flags, spec.Flag{Name: "--maybe", Value: tc.value, Choices: []string{"fast"}})
		result := resultFor(t, "acme copy "+tc.argument+" from to", snapshot)
		if result.Coverage == "checked" {
			t.Fatalf("uncertainty became a complete check: %+v", result)
		}
	}
}

func TestOptionalAttachmentRulesAreExplicit(t *testing.T) {
	for _, tc := range []struct{ value, command string }{
		{"optional-attached", "acme copy --color from to"},
		{"optional-attached", "acme copy --color=auto from to"},
		{"optional-separated", "acme copy --color auto from to"},
		{"optional-separated", "acme copy --color=auto from to"},
		{"optional-separated", "acme copy --color -- from to"},
	} {
		snapshot := contractFixture()
		snapshot.Root.Commands[0].Flags = append(snapshot.Root.Commands[0].Flags, spec.Flag{Name: "--color", Value: tc.value, Choices: []string{"auto"}})
		result := resultFor(t, tc.command, snapshot)
		if result.Coverage != "checked" || len(result.Diagnostics) != 0 {
			t.Fatalf("explicit optional-value rule failed: %+v", result)
		}
	}
}

func TestCheckedResultsContainNoUnresolvedFindings(t *testing.T) {
	for _, command := range []string{
		"acme copy a b", "acme copy a", "acme copy --mode=typo a b",
		"acme copy --mode=$MODE a b", "acme copy --force=maybe a b", "acme copy --unknown a b",
	} {
		for _, closed := range []bool{false, true} {
			snapshot := contractFixture()
			snapshot.Root.Commands[0].Closed = closed
			result := resultFor(t, command, snapshot)
			if result.Coverage == "checked" && slices.ContainsFunc(result.Diagnostics, func(d Diagnostic) bool { return d.Status != "confirmed" }) {
				t.Fatalf("checked result contains an unresolved rule: %+v", result)
			}
		}
	}
}
