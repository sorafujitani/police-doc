package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
)

func put(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scanFixture(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	fakeCLI(t, filepath.Join(dir, "bin/acme"), `case "$*" in
  --version) echo 'acme 1.0.0' ;;
  -h) printf 'Usage: acme COMMAND\nCommands:\n  status  Show status\n' ;;
  'status -h') printf 'Usage: acme status [OPTIONS]\n  --output FILE  Output file\n' ;;
  *) exit 10 ;;
esac
`)
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "# example\n\n```bash\n"+body+"\n```\n")
	return doc
}

func run(args ...string) (int, string, string) {
	var out, err bytes.Buffer
	code := Run(context.Background(), args, &out, &err, "test-version")
	return code, out.String(), err.String()
}

func TestScanExitCodesAndInterspersedOptions(t *testing.T) {
	for _, tc := range []struct {
		body, threshold string
		want            int
	}{
		{"acme status --output file", "error", 0},
		{"acme status --output", "error", 1},
		{"acme status --unknown", "error", 0},
		{"acme status --unknown", "warning", 1},
		{"acme status --output", "none", 0},
		{"acme status $DYNAMIC --output", "error", 0},
	} {
		t.Run(tc.body+"/"+tc.threshold, func(t *testing.T) {
			doc := scanFixture(t, tc.body)
			code, output, stderr := run("scan", doc, "--format", "json", "--fail-on", tc.threshold)
			if code != tc.want || stderr != "" {
				t.Fatalf("exit=%d stderr=%q output=%s", code, stderr, output)
			}
			var report check.Report
			if err := json.Unmarshal([]byte(output), &report); err != nil {
				t.Fatal(err)
			}
			if report.SchemaVersion != 2 || report.Summary.Files != 1 || report.Summary.Examples != 1 ||
				report.Summary.Partial != 1 || report.Results[0].Location.Line != 4 || report.Results[0].Version != "1.0.0" {
				t.Fatalf("unexpected report: %+v", report)
			}
		})
	}
}

func TestScanTextVerbosity(t *testing.T) {
	for _, tc := range []struct {
		body, option string
		verbose      bool
		wantCode     int
	}{
		{"acme status --output", "", false, 1},
		{"acme status --output", "--verbose", true, 1},
		{"acme status --output", "--verbose=false", false, 1},
		{"acme status --output file", "", false, 0},
	} {
		t.Run(tc.body+"/"+tc.option, func(t *testing.T) {
			doc := scanFixture(t, tc.body)
			args := []string{"scan", doc}
			if tc.option != "" {
				args = append(args, tc.option)
			}
			code, output, stderr := run(args...)
			if code != tc.wantCode || stderr != "" {
				t.Fatalf("exit=%d stderr=%q output=%s", code, stderr, output)
			}
			for _, detail := range []string{"INFO ", "Target:", "Evidence"} {
				if strings.Contains(output, detail) != tc.verbose {
					t.Fatalf("verbose=%v: unexpected visibility of %q: %s", tc.verbose, detail, output)
				}
			}
			if !strings.Contains(output, "Coverage: 1 partially checked, 0 uncheckable.") {
				t.Fatalf("hidden INFO must not hide coverage: %s", output)
			}
			if tc.wantCode == 0 && strings.Contains(output, tc.body) {
				t.Fatalf("INFO-only command should not be shown: %s", output)
			}
		})
	}
}

func TestScanCollectionFailureDetails(t *testing.T) {
	doc := scanFixture(t, "acme status")
	fakeCLI(t, filepath.Join(filepath.Dir(doc), "bin/acme"), "printf 'unsupported version probe\\nusage: acme [FLAGS]\\n'\nexit 2\n")
	for _, tc := range []struct {
		option string
		detail bool
	}{
		{"--format=text", false},
		{"--verbose", true},
		{"--format=json", true},
	} {
		t.Run(tc.option, func(t *testing.T) {
			code, output, stderr := run("scan", doc, "--fail-on=warning", tc.option)
			if code != 1 || stderr != "" || !strings.Contains(output, "help-unavailable") {
				t.Fatalf("collection failure was hidden: %d %s %s", code, output, stderr)
			}
			for _, detail := range []string{"unsupported version probe", "usage: acme [FLAGS]", "no wrapper was executed"} {
				if strings.Contains(output, detail) != tc.detail {
					t.Fatalf("unexpected detail visibility for %s: %s", tc.option, output)
				}
			}
		})
	}
}

func TestLegacyConfigurationIsIgnored(t *testing.T) {
	doc := scanFixture(t, "acme status --output")
	for _, path := range []string{"policedoc.yml", "policedoc.yaml"} {
		put(t, path, "not: [valid YAML")
	}
	code, output, stderr := run("scan", doc)
	if code != 1 || stderr != "" || !strings.Contains(output, "missing-flag-value") {
		t.Fatalf("legacy configuration interfered with discovery: %d %s %s", code, output, stderr)
	}
}

func TestScanNeverExecutesExamples(t *testing.T) {
	doc := scanFixture(t, `sh -c 'printf executed > "$POLICEDOC_TEST_MARKER"'
acme status "$(printf executed > "$POLICEDOC_TEST_MARKER")"`)
	marker := filepath.Join(filepath.Dir(doc), "NEVER_RUN")
	t.Setenv("POLICEDOC_TEST_MARKER", marker)
	fakeCLI(t, filepath.Join(filepath.Dir(doc), "bin/sh"), `case "$*" in
  --version) echo 'sh 1.0.0' ;;
  -h|--help) printf 'Usage: sh [OPTIONS]\n  -c COMMAND  Execute command\n' ;;
  *) printf executed > "$POLICEDOC_TEST_MARKER" ;;
esac
`)
	code, output, stderr := run("scan", doc, "--format=json")
	if code != 0 || stderr != "" || !strings.Contains(output, `"cli": "sh"`) || !strings.Contains(output, "dynamic-argument") {
		t.Fatalf("unexpected scan result %d %s %s", code, output, stderr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("document command ran: %v", err)
	}
}

func TestOperationalErrors(t *testing.T) {
	doc := scanFixture(t, "acme status")
	for _, args := range [][]string{
		{"unknown"}, {"scan"}, {"scan", doc, "--format=xml"},
		{"scan", doc, "--unknown"}, {"scan", doc, "--config", "policedoc.yml"},
		{"scan", doc, "--fail-on="}, {"scan", doc, "--fail-on=info"},
		{"scan", doc, "--cache-dir="}, {"scan", doc, "--offline"}, {"scan", doc, "--offline=false"},
		{"scan", doc, "--collect-help"}, {"scan", doc, "--collect-help=false"},
		{"scan", doc + ".missing"}, {"scan", doc, "--cache-dir"},
		{"collect-help"}, {"collect-help", "--help"}, {"version", "extra"},
	} {
		code, _, stderr := run(args...)
		if code != 2 || stderr == "" {
			t.Errorf("%v: exit=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestMarkdownDirectoryDiscovery(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"README.md", "docs/Nested.MD", "docs/page.markdown", "docs/plain.txt", ".git/hidden.md", "node_modules/pkg/ignored.md", "vendor/ignored.md"} {
		put(t, filepath.Join(dir, name), "text")
	}
	files, err := markdownFiles([]string{dir, filepath.Join(dir, "README.md")})
	if err != nil || len(files) != 3 {
		t.Fatalf("unexpected discovered files: %v %v", files, err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "alias.md")
		if err := os.Symlink(filepath.Join(dir, "README.md"), link); err != nil {
			t.Fatal(err)
		}
		if _, err := markdownFiles([]string{link}); err == nil {
			t.Fatal("symbolic link followed")
		}
	}
}

func TestHelpVersionAndOutputErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"scan", "--help"}, {"version"}, {"--version"}} {
		code, output, stderr := run(args...)
		if code != 0 || output == "" || stderr != "" {
			t.Errorf("%v: %d %q %q", args, code, output, stderr)
		}
	}
	if code, output, _ := run("version"); code != 0 || !strings.Contains(output, "policedoc test-version") {
		t.Fatal("wrong CLI name or version")
	}
	doc := scanFixture(t, "acme status")
	var stderr bytes.Buffer
	for _, args := range [][]string{nil, {"scan", "--help"}, {"version"}, {"scan", doc}} {
		if code := Run(context.Background(), args, brokenWriter{}, &stderr, "test"); code != 2 {
			t.Errorf("output failure ignored for %v", args)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := Run(ctx, []string{"scan", doc}, &bytes.Buffer{}, &stderr, "test"); code != 2 {
		t.Fatal("scan cancellation ignored")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }
