package check

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

func helpFixture() *spec.Snapshot {
	source := []spec.Evidence{{Kind: "help", Reference: "acme -h"}}
	return &spec.Snapshot{Tool: "acme", Version: "2.0.0", OS: "linux", Sources: source, Root: spec.Command{
		Sources: source,
		Flags:   []spec.Flag{{Name: "--root", Value: "unknown"}},
		Commands: []spec.Command{
			{Name: "publish", Sources: []spec.Evidence{{Kind: "help", Reference: "acme publish -h"}},
				Flags: []spec.Flag{
					{Name: "--output", Value: "required"}, {Name: "-o", Value: "required"},
					{Name: "--color", Value: "optional"}, {Name: "--switch", Value: "unknown"},
				}},
			{Name: "uncollected"},
		},
	}}
}

func resultFor(t *testing.T, command string, snapshot *spec.Snapshot) Result {
	t.Helper()
	if snapshot != nil {
		if err := snapshot.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	examples := extract.Markdown("README.md", []byte("```bash\n"+command+"\n```\n"))
	if len(examples) != 1 {
		t.Fatalf("expected one example: %#v", examples)
	}
	return Example(examples[0], snapshot)
}

func codes(result Result) []string {
	var codes []string
	for _, diagnostic := range result.Diagnostics {
		codes = append(codes, diagnostic.Code)
	}
	return codes
}

func TestHelpBasedChecks(t *testing.T) {
	for _, tc := range []struct {
		command, code, severity, status string
	}{
		{"acme publish --output file", "", "", ""},
		{"sudo -u root npx acme publish --output file", "", "", ""},
		{"./acme publish -o 'a b'", "", "", ""},
		{"acme publish --output=", "", "", ""},
		{"acme publish --output --legacy", "", "", ""},
		{"acme publish --color=always --output", "missing-flag-value", "error", "confirmed"},
		{"acme publish --output", "missing-flag-value", "error", "confirmed"},
		{"acme --root=yes publish --output", "missing-flag-value", "error", "confirmed"},
		{"acme publish --unknown --output", "unverified-flag", "warning", "needs-review"},
		{"acme publish --root", "unverified-flag", "warning", "needs-review"},
		{"acme publish --legacy", "unverified-flag", "warning", "needs-review"},
		{"acme publish --switch --output", "unknown-flag-arity", "warning", "uncheckable"},
		{"acme publish -- --output", "", "", ""},
		{"acme publish doc.md --output", "unchecked-arguments", "warning", "uncheckable"},
		{"acme unknown --output", "unchecked-arguments", "warning", "uncheckable"},
		{"acme publish $DOC --output", "dynamic-argument", "info", "uncheckable"},
		{"acme publish --output $OUT", "dynamic-argument", "info", "uncheckable"},
		{"acme publish *.md", "dynamic-argument", "info", "uncheckable"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			result := resultFor(t, tc.command, helpFixture())
			if result.Coverage != "partial" || result.Version != "2.0.0" {
				t.Fatalf("unexpected target or coverage: %+v", result)
			}
			if tc.code != "" && !slices.Contains(codes(result), tc.code) {
				t.Fatalf("missing %s: %+v", tc.code, result)
			}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Code == tc.code {
					if diagnostic.Severity != tc.severity || diagnostic.Status != tc.status || len(diagnostic.Evidence) == 0 {
						t.Errorf("unexpected diagnostic: %+v", diagnostic)
					}
				} else if diagnostic.Severity != "info" {
					t.Errorf("unexpected finding: %+v", diagnostic)
				}
			}
		})
	}
}

func TestUncheckableExamples(t *testing.T) {
	for _, tc := range []struct{ command, code string }{
		{"$CLI publish", "dynamic-command"},
		{"npx --package other acme publish", "unsupported-wrapper"},
		{"acme uncollected", "unavailable-spec"},
	} {
		result := resultFor(t, tc.command, helpFixture())
		if result.Coverage != "uncheckable" || !slices.Contains(codes(result), tc.code) {
			t.Errorf("%s: %+v", tc.command, result)
		}
	}
	result := resultFor(t, "acme publish", nil)
	if result.Coverage != "uncheckable" || !slices.Contains(codes(result), "unsupported-cli") {
		t.Fatalf("missing help was accepted: %+v", result)
	}
}

func TestReportAndExitThreshold(t *testing.T) {
	report := NewReport(1)
	report.Add(resultFor(t, "acme publish --unknown", helpFixture()))
	report.Add(resultFor(t, "acme publish $DOC", helpFixture()))
	if report.Fails("error") || !report.Fails("warning") || report.Fails("none") {
		t.Fatal("incorrect warning threshold")
	}
	report.Add(resultFor(t, "acme publish --output", helpFixture()))
	report.Add(resultFor(t, "acme publish", nil))
	if !report.Fails("error") || report.Fails("none") {
		t.Fatal("incorrect error threshold")
	}
	if report.Summary.Examples != report.Summary.Partial+report.Summary.Uncheckable {
		t.Fatal("coverage totals disagree")
	}
	for _, verbose := range []bool{false, true} {
		for _, format := range []string{"json", "text"} {
			var out bytes.Buffer
			if err := report.Write(&out, format, verbose); err != nil {
				t.Fatal(err)
			}
			if format == "json" {
				var decoded Report
				if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(report, &decoded) {
					t.Fatal("JSON lost report data")
				}
				continue
			}
			for _, detail := range []string{"INFO incomplete-spec", "Target:", "Evidence", "acme publish -h", "[confirmed]"} {
				if bytes.Contains(out.Bytes(), []byte(detail)) != verbose {
					t.Fatalf("verbose=%v: unexpected visibility of %q: %s", verbose, detail, out.String())
				}
			}
			wantLocations := 3
			if verbose {
				wantLocations = 4
			}
			if bytes.Count(out.Bytes(), []byte("README.md:2:1")) != wantLocations {
				t.Fatalf("expected one location per visible command: %s", out.String())
			}
			for _, finding := range []string{"ERROR missing-flag-value", "WARNING unverified-flag", "INFO dynamic-argument", "acme publish $DOC", "1 files, 4 examples: 1 errors, 1 warnings", "Coverage: 3 partially checked, 1 uncheckable."} {
				if !bytes.Contains(out.Bytes(), []byte(finding)) {
					t.Fatalf("text lost %q: %s", finding, out.String())
				}
			}
		}
		var empty bytes.Buffer
		if err := NewReport(0).Write(&empty, "text", verbose); err != nil || !bytes.Contains(empty.Bytes(), []byte("No supported shell examples")) {
			t.Fatal("empty scan not identified")
		}
	}
}
