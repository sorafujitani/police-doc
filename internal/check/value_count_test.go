package check

import (
	"slices"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestMultipleValueBoundaries(t *testing.T) {
	snapshot := helpFixture()
	command := snapshot.Root.Child("publish")
	command.Flags = append(command.Flags, spec.Flag{Name: "--pair", Value: "required", ValueCount: 2})
	for _, tc := range []struct {
		command string
		want    []string
		dynamic bool
	}{
		{"acme publish --pair", []string{"missing-flag-value"}, false},
		{"acme publish --pair one", []string{"missing-flag-value"}, false},
		{"acme publish --pair one two --output file", nil, false},
		{"acme publish --pair '' two --output file", nil, false},
		{"acme publish --pair one two --output", []string{"missing-flag-value"}, false},
		{"acme publish --pair one two --unknown", []string{"unverified-flag"}, false},
		{"acme publish --pair one $TWO --unknown", nil, true},
		{"acme publish --pair $VALUES", nil, true},
		{"acme publish --pair=one two --unknown", []string{"unknown-flag-arity"}, false},
	} {
		result := resultFor(t, tc.command, snapshot)
		var findings []string
		dynamic := false
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Severity != "info" {
				findings = append(findings, diagnostic.Code)
			}
			dynamic = dynamic || diagnostic.Code == "dynamic-argument"
		}
		if !slices.Equal(findings, tc.want) || dynamic != tc.dynamic {
			t.Errorf("%s produced %v (dynamic %t), want %v (dynamic %t)", tc.command, findings, dynamic, tc.want, tc.dynamic)
		}
	}
}
