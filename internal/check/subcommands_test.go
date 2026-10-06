package check

import (
	"slices"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestUnlistedSubcommandsOnlyWarnAtKnownCommandPositions(t *testing.T) {
	for _, tc := range []struct {
		command string
		warn    bool
	}{
		{"acme publis", true},
		{"acme --config file publis", true},
		{"acme --config=file publis", true},
		{"acme publish", false},
		{"acme publish document.txt --output", false},
		{"acme publish publis", false},
		{"acme --config publis", false},
		{"acme --root publis", false},
		{"acme -- publis", false},
		{"acme $COMMAND publis", false},
		{"acme uncollected publis", false},
	} {
		t.Run(tc.command, func(t *testing.T) {
			snapshot := helpFixture()
			snapshot.Root.SubcommandFirst = true
			snapshot.Root.Flags = append(snapshot.Root.Flags, spec.Flag{Name: "--config", Value: "required"})
			result := resultFor(t, tc.command, snapshot)
			if got := slices.Contains(codes(result), "unverified-command"); got != tc.warn {
				t.Fatalf("warn=%v, want=%v: %+v", got, tc.warn, result)
			}
		})
	}
	for _, commandFirst := range []bool{false, true} {
		snapshot := helpFixture()
		snapshot.Root.SubcommandFirst = commandFirst
		if commandFirst {
			snapshot.Root.Commands = nil // COMMAND may be an executable to forward to.
		}
		result := resultFor(t, "acme arbitrary", snapshot)
		if slices.Contains(codes(result), "unverified-command") {
			t.Fatalf("usage and command-list evidence are both required: %+v", result)
		}
	}
}
