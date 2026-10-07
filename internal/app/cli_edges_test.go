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

func TestHelpDrivenCLIGrammar(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	log := filepath.Join(dir, "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	fakeCLI(t, filepath.Join(dir, "bin/acme"), `
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 1.0.0 ;;
  -h) printf 'Usage: acme [-v | --version]\n           [--config FILE]\n           <command> [<args>]\n\nCORE COMMANDS\n  pr:  Pull requests\n  search:  Search files\n' ;;
  'pr -h') printf 'USAGE\n  acme pr <command> [flags]\n\nAVAILABLE COMMANDS\n  list:  List requests\n  view:  View a request\n' ;;
  'pr list -h') printf 'USAGE\n  acme pr list [flags]\n\nALIASES\n  acme pr ls\n\nFLAGS\n  -d, --draft  Filter drafts\n  -L, --limit int  Number of results\n  -s, --state string  Filter by state: {open|closed|merged|all}\n' ;;
  'pr view -h') printf 'USAGE\n  acme pr view [<number> | <url> | <branch>] [flags]\n\nFLAGS\n  -c, --comments  Show comments\n' ;;
  'search -h') printf 'Usage: acme search [OPTIONS] PATTERN [PATH...]\n\nOptions:\n  -i, --ignore-case  Ignore case\n  -n, --line-number  Show line numbers\n  -e, --regexp PATTERN  Search pattern\n' ;;
  *) echo 'error: document arguments must never be executed'; exit 9 ;;
esac
`)
	commands := []string{
		"acme sttaus", "acme pr list --draft --limt 5", "acme pr view 123 --commnts",
		"acme pr list --state opne", "acme pr ls", "acme search -in pattern .",
		"acme pr list --state=open --draft", "acme pr view 123 --comments",
		"acme search -inepattern .", "acme pr ls --limit 5",
	}
	want := []string{"unverified-command", "unverified-flag", "unverified-flag", "unverified-flag-value", "", "", "", "", "", ""}
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "```sh\n"+strings.Join(commands, "\n")+"\n```\n")
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
		if code != 1 || stderr != "" || report.Summary.Errors != 0 || report.Summary.Warnings != 4 || len(report.Results) != len(commands) {
			t.Fatalf("%s: %d %s %s", mode, code, output, stderr)
		}
		for i, result := range report.Results {
			var found []string
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Severity != "info" {
					found = append(found, diagnostic.Code)
				}
			}
			if (want[i] == "" && len(found) != 0) || (want[i] != "" && !slices.Equal(found, []string{want[i]})) {
				t.Errorf("%s: %s produced %v, want %q", mode, commands[i], found, want[i])
			}
		}
		calls, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "cached" && string(calls) != "--version\n" {
			t.Fatalf("cached aliases caused help requests: %q", calls)
		}
		for call := range strings.SplitSeq(strings.TrimSpace(string(calls)), "\n") {
			if !slices.Contains([]string{"--version", "-h", "pr -h", "pr list -h", "pr view -h", "search -h"}, call) {
				t.Fatalf("unsafe/unnecessary help request: %q", call)
			}
		}
		put(t, log, "")
	}
}
