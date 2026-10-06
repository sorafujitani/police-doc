package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
)

func lazyHelpFixture(t *testing.T) (doc, log string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	log = filepath.Join(dir, "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	fakeCLI(t, filepath.Join(dir, "bin/acme"), `
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 1.0.0 ;;
  -h)
    printf 'Usage: acme COMMAND\nOptions:\n  --config FILE  Configuration\n  --toggle  Unknown arity\nCommands:\n'
    i=0
    while [ "$i" -lt 150 ]; do
      printf '  unused%s  Unrelated command\n' "$i"
      i=$((i+1))
    done
    printf '  export  Export data\n  group  Nested commands\n  broken  Unavailable help\n  wrong  Returns parent help\n  example  Returns unrelated examples\n  deeper  Returns descendant help\n'
    ;;
  'export -h') printf 'Usage: acme export [OPTIONS]\n  --output FILE  Output file\n' ;;
  'group -h')
    [ "$POLICEDOC_TEST_GROUP_DOWN" = yes ] && exit 3
    printf 'Usage: acme group COMMAND\nCommands:\n  leaf  Nested command\n' ;;
  'group leaf -h') printf 'Usage: acme group leaf [OPTIONS]\n  --destination FILE  Output file\n' ;;
  broken*)
    if [ "$POLICEDOC_TEST_READY" = yes ]; then
      printf 'Usage: acme broken\n'
    else
      exit 3
    fi ;;
  wrong*) printf 'Usage: acme COMMAND\n  --output FILE  Parent flag\n\nFurther help:\n  acme wrong [OPTIONS]\n' ;;
  example*) printf 'Example usage:\n  acme other\nOptions:\n  --output FILE  Unrelated flag\n' ;;
  deeper*) printf 'Usage: acme deeper nested [OPTIONS]\n  --output FILE  Descendant flag\n' ;;
  *) echo 'error: an unrelated command was requested'; exit 9 ;;
esac
`)
	return filepath.Join(dir, "README.md"), log
}

func scanReport(t *testing.T, doc string) check.Report {
	t.Helper()
	code, output, stderr := run("scan", doc, "--format=json")
	var report check.Report
	if err := json.Unmarshal([]byte(output), &report); err != nil || stderr != "" || code == 2 {
		t.Fatalf("scan failed: exit=%d stderr=%s output=%s error=%v", code, stderr, output, err)
	}
	return report
}

func TestHelpFollowsOnlyCheckableDocumentPaths(t *testing.T) {
	doc, log := lazyHelpFixture(t)
	put(t, doc, "```sh\n"+strings.Join([]string{
		"acme export --output result.txt",
		"acme --config file export --output",
		"acme --toggle unused0",
		"acme input-file unused1",
		"acme -- unused2",
		"acme \"$DYNAMIC\" unused3",
	}, "\n")+"\n```\n")
	report := scanReport(t, doc)
	if report.Summary.Errors != 1 || report.Summary.Warnings != 0 || report.Summary.Uncheckable != 0 {
		t.Fatalf("unrelated help affected results: %+v", report)
	}
	assertHelpCalls(t, log, "--version\n-h\nexport -h\n")

	// A new example expands the cached tree without recollecting its root or
	// visiting other commands. Dynamic words, values and forwarded tails never
	// become help requests.
	put(t, doc, "```sh\nacme export --output result.txt\nacme group leaf --destination\n```\n")
	report = scanReport(t, doc)
	if report.Summary.Errors != 1 || report.Summary.Warnings != 0 || report.Summary.Uncheckable != 0 {
		t.Fatalf("cached help was not extended: %+v", report)
	}
	assertHelpCalls(t, log, "--version\ngroup -h\ngroup leaf -h\n")
	_, first, _ := run("scan", doc, "--format=json")
	_, second, _ := run("scan", doc, "--format=json")
	if first != second {
		t.Fatal("cached verdicts changed")
	}
	assertHelpCalls(t, log, "--version\n--version\n")
}

func TestHelpFailuresAreLocalAndRetriedNextScan(t *testing.T) {
	doc, log := lazyHelpFixture(t)
	t.Setenv("POLICEDOC_TEST_READY", "no")
	put(t, doc, "```sh\nacme export --output file\nacme broken\nacme broken\nacme export --output file\n```\n")
	report := scanReport(t, doc)
	if report.Summary.Warnings != 2 || report.Summary.Uncheckable != 2 || report.Summary.Errors != 0 {
		t.Fatalf("wrong failure scope: %+v", report)
	}
	for _, i := range []int{0, 3} {
		for _, d := range report.Results[i].Diagnostics {
			if d.Severity == "warning" {
				t.Fatalf("unrelated warning: %+v", report.Results[i])
			}
		}
	}
	assertHelpCalls(t, log, "--version\n-h\nexport -h\nbroken -h\nbroken --help\n")
	for _, path := range cacheEntries(t, filepath.Dir(doc)) {
		data, err := os.ReadFile(path)
		if err != nil || strings.Contains(string(data), "exit status") {
			t.Fatalf("failure was cached: %s %v", data, err)
		}
	}

	t.Setenv("POLICEDOC_TEST_READY", "yes")
	report = scanReport(t, doc)
	if report.Summary.Warnings != 0 || report.Summary.Uncheckable != 0 {
		t.Fatalf("failed help was not retried: %+v", report)
	}
	assertHelpCalls(t, log, "--version\nbroken -h\n")
}

func TestWrongHelpCannotValidateCommandFlags(t *testing.T) {
	for _, name := range []string{"wrong", "example", "deeper"} {
		t.Run(name, func(t *testing.T) {
			doc, log := lazyHelpFixture(t)
			put(t, doc, "```sh\nacme "+name+" --output\nacme export --output file\n```\n")
			report := scanReport(t, doc)
			if report.Summary.Errors != 0 || report.Summary.Warnings != 1 || report.Results[0].Coverage != "uncheckable" || report.Results[1].Coverage != "partial" {
				t.Fatalf("unrelated help was treated as command evidence: %+v", report)
			}
			assertHelpCalls(t, log, "--version\n-h\n"+name+" -h\n"+name+" --help\nexport -h\n")
			for _, d := range report.Results[0].Diagnostics {
				for _, evidence := range d.Evidence {
					if strings.Contains(evidence.Reference, "export") {
						t.Fatal("unrelated cached evidence was attached to a failed command")
					}
				}
			}
		})
	}
}

func TestSharedExecutableCacheAcrossNpxOrigins(t *testing.T) {
	doc, log := lazyHelpFixture(t)
	dir := filepath.Dir(doc)
	a, b := filepath.Join(dir, "one/README.md"), filepath.Join(dir, "two/README.md")
	put(t, a, "```sh\nnpx acme export --output file\n```\n")
	put(t, b, "```sh\nnpx acme group leaf --destination file\n```\n")
	t.Setenv("POLICEDOC_TEST_GROUP_DOWN", "no")
	if code, out, stderr := run("scan", a, b, "--fail-on=warning"); code != 0 || stderr != "" {
		t.Fatalf("first scan: %d %s %s", code, out, stderr)
	}
	assertHelpCalls(t, log, "--version\n-h\nexport -h\ngroup -h\ngroup leaf -h\n")
	t.Setenv("POLICEDOC_TEST_GROUP_DOWN", "yes")
	if code, out, stderr := run("scan", a, b, "--fail-on=warning"); code != 0 || stderr != "" {
		t.Fatalf("shared successful help was lost: %d %s %s", code, out, stderr)
	}
	assertHelpCalls(t, log, "--version\n")
}
