package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sorafujitani/police-doc/internal/check"
)

// Opt in with POLICEDOC_REAL_CLI_TESTS=1; only installed, trusted CLIs are used.
func TestRealCLIHelpAndScan(t *testing.T) {
	if os.Getenv("POLICEDOC_REAL_CLI_TESTS") != "1" {
		t.Skip("set POLICEDOC_REAL_CLI_TESTS=1 to collect installed CLI help")
	}
	for _, tc := range []struct{ tool, good, missingValue string }{
		{"go", "go build -o example", "go build -o"},
		{"git", "git commit --file=message.txt", "git commit --file"},
		{"node", `node --eval "console.log(1)"`, "node --eval"},
		{"python3", `python3 -c "print(1)"`, ""},
		{"uv", "uv --quiet --version", "uv pip install --python"},
		{"rg", "rg -e pattern .", "rg -e"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			if _, err := exec.LookPath(tc.tool); err != nil {
				t.Skipf("%s is not installed", tc.tool)
			}
			dir := t.TempDir()
			t.Chdir(dir)
			doc := filepath.Join(dir, "usage.md")
			body := tc.good + "\n" + tc.tool + " --policedoc-unknown-flag\n"
			wantExit, wantErrors, wantExamples := 0, 0, 2
			if tc.missingValue != "" {
				body += tc.missingValue + "\n"
				wantExit, wantErrors, wantExamples = 1, 1, 3
			}
			put(t, doc, "```sh\n"+body+"```\n")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var out, stderr bytes.Buffer
			code := Run(ctx, []string{"scan", doc, "--format=json"}, &out, &stderr, "test")
			if code != wantExit || stderr.Len() != 0 {
				t.Fatalf("scan: exit=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
			}
			var report check.Report
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Summary.Errors != wantErrors || report.Summary.Partial != wantExamples || report.Summary.Uncheckable != 0 {
				t.Fatalf("unexpected summary: %+v\n%s", report.Summary, out.String())
			}
			for _, result := range report.Results {
				if result.CLI != tc.tool || result.Version == "" || result.Location.File != doc {
					t.Fatalf("unexpected provenance: %+v", result)
				}
			}
			if !slices.ContainsFunc(report.Results[1].Diagnostics, func(d check.Diagnostic) bool {
				return d.Code == "unverified-flag" && d.Status == "needs-review"
			}) {
				t.Fatal("unknown flag was not reported for review")
			}
			if tc.missingValue != "" && !slices.ContainsFunc(report.Results[2].Diagnostics, func(d check.Diagnostic) bool {
				return d.Code == "missing-flag-value" && d.Status == "confirmed"
			}) {
				t.Fatalf("missing value was not confirmed: %+v", report.Results[2])
			}
			code, cached, errors := run("scan", doc, "--format=json")
			if code != wantExit || errors != "" || cached != out.String() || strings.Contains(cached, "help-unavailable") {
				t.Fatalf("cached scan changed the verdict: %d %s %s", code, cached, errors)
			}
			t.Logf("%s %s: automatically collected and scanned %d examples, then repeated with cached help", tc.tool, report.Results[0].Version, wantExamples)
		})
	}
}
