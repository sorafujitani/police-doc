package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
)

func TestSubcommandTyposAtEveryLevel(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	log := filepath.Join(dir, "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	fakeCLI(t, filepath.Join(dir, "bin/acme"), `
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 1.0.0 ;;
  -h) printf 'USAGE\n  acme <command> <subcommand> [flags]\n\nCORE COMMANDS\n  auth:  Authenticate\n\nFLAGS\n  --help  Show help\n' ;;
  'auth -h') printf 'USAGE\n  acme auth <command> [flags]\n\nAVAILABLE COMMANDS\n  login:  Log in\n\nFLAGS\n  --help  Show help\n' ;;
  'auth login -h') printf 'USAGE\n  acme auth login [flags]\n\nFLAGS\n  --hostname HOST  Host\n' ;;
  *) echo 'error: unadvertised command was executed'; exit 9 ;;
esac
`)
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "```sh\nacme auth login\n```\n")
	if report := scanReport(t, doc); report.Summary.Warnings != 0 {
		t.Fatalf("valid command warned: %+v", report)
	}
	assertHelpCalls(t, log, "--version\n-h\nauth -h\nauth login -h\n")

	// Editing a document must be checked even when the CLI's help is cached.
	put(t, doc, "```sh\nacme auth login\nacme aut login\nacme auth logi\n```\n")
	for _, refresh := range []bool{false, true} {
		args := []string{"scan", doc, "--format=json", "--fail-on=warning"}
		if refresh {
			args = append(args, "--refresh")
		}
		code, output, stderr := run(args...)
		var report check.Report
		if err := json.Unmarshal([]byte(output), &report); err != nil {
			t.Fatal(err)
		}
		if code != 1 || stderr != "" || report.Summary.Warnings != 2 || report.Summary.Errors != 0 || report.Summary.Partial != 3 {
			t.Fatalf("refresh=%v: exit=%d report=%+v stderr=%s", refresh, code, report, stderr)
		}
		for i, parent := range []string{"acme -h", "acme auth -h"} {
			result := report.Results[i+1]
			if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "unverified-command" || result.Diagnostics[0].Status != "needs-review" {
				t.Fatalf("typo was not reported: %+v", result)
			}
			evidence := result.Diagnostics[0].Evidence
			if len(evidence) != 1 || !strings.HasSuffix(evidence[0].Reference, parent) {
				t.Fatalf("wrong parent evidence: %+v", evidence)
			}
		}
		want := "--version\n"
		if refresh {
			want += "-h\nauth -h\nauth login -h\n"
		}
		assertHelpCalls(t, log, want)
	}
	code, output, stderr := run("scan", doc)
	if code != 0 || stderr != "" || strings.Count(output, "WARNING unverified-command") != 2 {
		t.Fatalf("warnings missing from default output: %d %s %s", code, output, stderr)
	}
}
