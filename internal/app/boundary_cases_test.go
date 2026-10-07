package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
)

func TestAuditedHelpAndShellBoundaries(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	log := filepath.Join(dir, "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	fakeCLI(t, filepath.Join(dir, "bin/edgeprobe"), `
printf 'edgeprobe %s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 'edgeprobe 1.0.0' ;;
  -h) printf '%s\n' 'Usage: edgeprobe [OPTIONS]
Options:
  -p, --pair LEFT RIGHT  Two required values
  --point <X,Y>  A coordinate pair
  --template TEXT  Pattern, e.g.: {title|author}
  -v, --verbose  Verbose mode' ;;
  *) echo 'error: non-metadata invocation'; exit 9 ;;
esac
`)
	fakeCLI(t, filepath.Join(dir, "bin/catalogprobe"), `
printf 'catalogprobe %s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 'catalogprobe 1.0.0' ;;
  -h) printf '%s\n' 'Usage: catalogprobe <command> [flags]
Commands:
  alpha   First command
  export  Export supported formats:
      csv   Comma-separated records:
  status  Show status
Flags:
  -h, --help  Show help' ;;
  'alpha -h'|'export -h'|'status -h') printf 'Usage: catalogprobe %s [flags]\n\nFlags:\n  --verbose  Verbose mode\n' "$1" ;;
  *) echo 'error: non-metadata invocation'; exit 9 ;;
esac
`)
	fakeCLI(t, filepath.Join(dir, "bin/FOO=bar"), `echo forbidden >> "$POLICEDOC_TEST_CALLS"; exit 9`)
	cases := []struct{ command, diagnostic string }{
		{"edgeprobe --pair one", "missing-flag-value"},
		{"edgeprobe --pair one two --verbose", ""},
		{"edgeprobe --pair=one two --verbose", "unknown-flag-arity"},
		{"edgeprobe -pone two --verbose", "unknown-flag-arity"},
		{"edgeprobe --pair one two --typo", "unverified-flag"},
		{"edgeprobe --point 10,20", ""},
		{"edgeprobe --template '{name}'", ""},
		{"PATH=/missing edgeprobe --verbose", "unsupported-environment"},
		{`./'fake\npx' edgeprobe --verbose`, "help-unavailable"},
		{"sudo FOO=bar edgeprobe --verbose", "unsupported-environment"},
		{"sudo -- FOO=bar edgeprobe --verbose", "unsupported-environment"},
		{"sudo -- edgeprobe --verbose", ""},
		{"npx edgeprobe --verbose", ""},
		{"catalogprobe status --verbose", ""},
		{"catalogprobe export --verbose", ""},
	}
	var lines []string
	for _, tc := range cases {
		lines = append(lines, tc.command)
	}
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "```bash\n"+strings.Join(lines, "\n")+"\n```\n")
	for _, mode := range []string{"fresh", "cached", "refresh"} {
		args := []string{"scan", doc, "--format=json", "--fail-on=warning"}
		if mode == "refresh" {
			args = append(args, "--refresh")
		}
		code, output, stderr := run(args...)
		var report check.Report
		if err := json.Unmarshal([]byte(output), &report); err != nil {
			t.Fatal(err)
		}
		if code != 1 || stderr != "" || len(report.Results) != len(cases) {
			t.Fatalf("%s: %d %s %s", mode, code, output, stderr)
		}
		for i, result := range report.Results {
			var got, want []string
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Severity != "info" {
					got = append(got, diagnostic.Code)
				}
			}
			if cases[i].diagnostic != "" {
				want = []string{cases[i].diagnostic}
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s: %s produced %v, want %v", mode, cases[i].command, got, want)
			}
		}
		calls, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		for call := range strings.SplitSeq(strings.TrimSpace(string(calls)), "\n") {
			if !slices.Contains([]string{"edgeprobe --version", "edgeprobe -h", "catalogprobe --version", "catalogprobe -h", "catalogprobe alpha -h", "catalogprobe export -h", "catalogprobe status -h"}, call) {
				t.Fatalf("unexpected invocation: %q", call)
			}
		}
		put(t, log, "")
	}
}
