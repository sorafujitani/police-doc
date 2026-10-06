package check

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestShortFlagBoundaries(t *testing.T) {
	command := spec.Command{ShortFlagClusters: true, Flags: []spec.Flag{
		{Name: "-i", Value: "none"}, {Name: "-n", Value: "none"},
		{Name: "-e", Value: "required"}, {Name: "-c", Value: "optional"},
		{Name: "-x", Value: "unknown"}, {Name: "-race", Value: "none"},
	}}
	for _, tc := range []struct {
		word string
		want []string
	}{
		{"-in", []string{"-i", "-n"}}, {"-inepattern", []string{"-i", "-n", "-e=pattern"}},
		{"-ne", []string{"-n", "-e"}}, {"-e--unknown", []string{"-e=--unknown"}},
		{"-icblue", []string{"-i", "-c=blue"}}, {"-ine=pattern", []string{"-i", "-n", "-e=pattern"}},
		{"-inx", []string{"-inx"}}, {"-inz", []string{"-inz"}},
		{"-race", []string{"-race"}}, {"--unknown", []string{"--unknown"}},
	} {
		if got := flagWords(&command, tc.word); !slices.Equal(got, tc.want) {
			t.Errorf("%s -> %v, want %v", tc.word, got, tc.want)
		}
	}
	command.ShortFlagClusters = false
	if got := flagWords(&command, "-in"); !slices.Equal(got, []string{"-in"}) {
		t.Fatalf("non-getopt syntax was expanded: %v", got)
	}
	command.ShortFlagClusters = true
	command.Flags = append(command.Flags, spec.Flag{Name: "-in", Value: "required"})
	if got := flagWords(&command, "-in"); !slices.Equal(got, []string{"-in"}) {
		t.Fatalf("exact single-dash long flag lost precedence: %v", got)
	}
}

func TestChoicesAndTrailingFlags(t *testing.T) {
	snapshot := helpFixture()
	command := snapshot.Root.Child("publish")
	command.FlagsAfterPositionals = true
	command.ShortFlagClusters = true
	command.Flags = append(command.Flags,
		spec.Flag{Name: "-s", Value: "required", Choices: []string{"open", "closed"}},
		spec.Flag{Name: "--draft", Value: "none"},
	)
	for _, tc := range []struct{ command, finding string }{
		{"acme publish --draft --typo", "unverified-flag"},
		{"acme publish file --typo", "unverified-flag"},
		{"acme publish -sopne", "unverified-flag-value"},
		{"acme publish -s opne", "unverified-flag-value"},
		{"acme publish -s=open", ""},
		{"acme publish -sopen", ""},
		{"acme publish file other --output", "missing-flag-value"},
		{"acme publish file -- --typo", ""},
	} {
		result := resultFor(t, tc.command, snapshot)
		var findings []string
		for _, d := range result.Diagnostics {
			if d.Severity != "info" {
				findings = append(findings, d.Code)
			}
		}
		if (tc.finding == "" && len(findings) != 0) || (tc.finding != "" && !slices.Equal(findings, []string{tc.finding})) {
			t.Errorf("%s produced %v, want %q", tc.command, findings, tc.finding)
		}
	}
}

func TestStoppedChecksAreVisibleWithoutVerbose(t *testing.T) {
	for _, command := range []string{
		"acme publish --switch --output", "acme publish script --unknown", "acme publish $DYNAMIC --output", "acme publish -- --unknown",
	} {
		report := NewReport(1)
		report.Add(resultFor(t, command, helpFixture()))
		var text bytes.Buffer
		if err := report.Write(&text, "text", false); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text.String(), command) || !strings.Contains(text.String(), "not checked") {
			t.Fatalf("stopped check was hidden: %s", text.String())
		}
	}
}
