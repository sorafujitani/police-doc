package check

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestUncollectedHelpDoesNotWarnAboutFlags(t *testing.T) {
	result := resultFor(t, "acme uncollected --anything", helpFixture())
	if result.Coverage != "uncheckable" || !slices.Contains(codes(result), "unavailable-spec") {
		t.Fatalf("missing unavailable coverage: %+v", result)
	}
	report := NewReport(1)
	report.Add(result)
	if report.Fails("warning") {
		t.Fatalf("missing help was treated as evidence against a flag: %+v", result.Diagnostics)
	}
}

func TestOptionalFlagDoesNotGuessSubcommand(t *testing.T) {
	snapshot := helpFixture()
	snapshot.Root.Flags = append(snapshot.Root.Flags, spec.Flag{Name: "--profile", Value: "optional"})
	for _, command := range []string{"acme --profile publish --output", "acme publish --color --output"} {
		result := resultFor(t, command, snapshot)
		if slices.Contains(codes(result), "missing-flag-value") {
			t.Fatalf("ambiguous flag value caused a confirmed error: %+v", result.Diagnostics)
		}
	}
	result := resultFor(t, "acme --profile=work publish --output", snapshot)
	if !slices.Contains(codes(result), "missing-flag-value") {
		t.Fatalf("explicit flag value prevented checking the tail: %+v", result)
	}
}

func TestAmbiguousConsoleDoesNotWarn(t *testing.T) {
	examples := extract.Markdown("doc.md", []byte("```console\n$ printf '%s' '\n$ value\n> value\n'\n```"))
	if len(examples) != 1 {
		t.Fatalf("ambiguous console produced commands: %+v", examples)
	}
	report := NewReport(1)
	report.Add(Example(examples[0], helpFixture()))
	if report.Summary.Uncheckable != 1 || report.Fails("warning") {
		t.Fatalf("ambiguous console was treated as invalid shell: %+v", report)
	}
}

func TestReportEscapesTerminalControls(t *testing.T) {
	untrusted := "café\x1b[2J\r\a\n\t\u0085\u202e"
	report := NewReport(1)
	report.Add(Result{
		Location: extract.Location{File: untrusted, Line: 2, Column: 1},
		Command:  untrusted, CLI: untrusted, Version: untrusted, OS: untrusted,
		Coverage: "uncheckable",
		Diagnostics: []Diagnostic{{
			Severity: "warning", Status: "needs-review", Code: "unverified-flag",
			Message: untrusted, Detail: untrusted, Evidence: []spec.Evidence{{Kind: "help", Reference: untrusted, Detail: untrusted}},
		}},
	})
	var out bytes.Buffer
	for _, verbose := range []bool{false, true} {
		out.Reset()
		if err := report.Write(&out, "text", verbose); err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(out.String(), "\x1b\r\a\t\u0085\u202e") ||
			!strings.Contains(out.String(), "café\\x1b[2J\\r\\a\\n\\t\\u0085\\u202e") {
			t.Fatalf("terminal controls were not escaped: %q", out.String())
		}
	}
	out.Reset()
	if err := report.Write(&out, "json", false); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Results[0].Command != untrusted || decoded.Results[0].Diagnostics[0].Detail != untrusted {
		t.Fatal("JSON must retain the original data")
	}
}
