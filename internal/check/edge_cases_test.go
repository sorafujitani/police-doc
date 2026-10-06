package check

import (
	"slices"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestAttachedOptionalValuesDoNotConsumeFollowingArguments(t *testing.T) {
	snapshot := helpFixture()
	command := snapshot.Root.Child("publish")
	command.ShortFlagClusters = true
	color := command.Flag("--color")
	color.Value, color.Choices = "optional-attached", []string{"always", "never"}
	command.Flags = append(command.Flags, spec.Flag{Name: "-c", Value: "optional-attached", Choices: color.Choices})
	for _, tc := range []struct {
		command string
		want    []string
	}{
		{"acme publish --color --output", []string{"missing-flag-value"}},
		{"acme publish --color --unknown", []string{"unverified-flag"}},
		{"acme publish --color=never --output file", nil},
		{"acme publish --color=typo --output file", []string{"unverified-flag-value"}},
		{"acme publish -c --output", []string{"missing-flag-value"}},
		{"acme publish -cnever --output file", nil},
		{"acme publish --color -- --unknown", nil},
	} {
		result := resultFor(t, tc.command, snapshot)
		var findings []string
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Severity != "info" {
				findings = append(findings, diagnostic.Code)
			}
		}
		if !slices.Equal(findings, tc.want) {
			t.Errorf("%s produced %v, want %v", tc.command, findings, tc.want)
		}
	}
}
