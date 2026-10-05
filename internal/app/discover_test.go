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
	"time"

	"github.com/sorafujitani/police-doc/internal/check"
)

func fakeCLI(t *testing.T, path, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("The fake CLI uses a POSIX shell script.")
	}
	put(t, path, "#!/bin/sh\n"+script)
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverAndScan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	bin := filepath.Join(dir, "bin")
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", bin)
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	fakeCLI(t, filepath.Join(bin, "atlas"), `
printf 'atlas %s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 'atlas 1.2.3' ;;
  --help) printf 'Usage: atlas COMMAND\nCommands:\n  export  Export data\n' ;;
  'export -h') printf 'Usage: atlas export [OPTIONS]\n  --output FILE  Output file\n' ;;
  -h) printf 'error: use --help\n'; exit 2 ;;
  *) exit 20 ;;
esac
`)
	fakeCLI(t, filepath.Join(dir, "node_modules/.bin/linty"), `
printf 'linty %s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 'linty 3.4.5' ;;
  -h) printf 'Usage: linty [OPTIONS]\n  --output FILE  Output file\n' ;;
  *) exit 21 ;;
esac
`)
	for _, wrapper := range []string{"sudo", "npx"} {
		fakeCLI(t, filepath.Join(bin, wrapper), `printf 'WRAPPER WAS EXECUTED\n' >> "$POLICEDOC_TEST_CALLS"; exit 22`)
	}
	doc := filepath.Join(dir, "docs/README.md")
	put(t, doc, "```sh\nsudo atlas export --output result.txt\nsudo -u root atlas export --output\nnpx linty --output result.txt\nnpx --no-install linty --output\nsudo npx linty --output result.txt\nnpx --package unwanted linty .\n```\n")
	code, output, stderr := run("scan", doc, "--format=json")
	var report check.Report
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("invalid report: %s %s %v", output, stderr, err)
	}
	if code != 1 || stderr != "" || report.Summary.Errors != 2 || report.Summary.Warnings != 0 || report.Summary.Uncheckable != 1 || report.Results[0].CLI != "atlas" || report.Results[2].CLI != "linty" || report.Results[2].Version != "3.4.5" {
		t.Fatalf("unexpected scan: %d %s %s", code, output, stderr)
	}
	calls, err := os.ReadFile(log)
	want := "atlas --version\natlas -h\natlas --help\natlas export -h\nlinty --version\nlinty -h\n"
	if err != nil || string(calls) != want {
		t.Fatalf("wrong calls, repeated collection or wrapper execution: %q %v", calls, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "policedoc.yml")); !os.IsNotExist(err) {
		t.Fatal("scan unexpectedly wrote a configuration")
	}
	// Cached scanning still checks versions, but does not recollect unchanged help.
	code, output, stderr = run("scan", doc, "--format=json")
	after, err := os.ReadFile(log)
	if code != 1 || stderr != "" || strings.Contains(output, "help-unavailable") || err != nil || string(after) != want+"atlas --version\nlinty --version\n" {
		t.Fatalf("cached scan failed or recollected help: %d %s %q %v", code, stderr, after, err)
	}
}

func TestDiscoveryKeepsProjectLocalVersionsSeparate(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", t.TempDir())
	for _, entry := range []struct{ folder, version, flag, value string }{
		{"one", "1.0.0", "--alpha", "ok"}, {"two", "2.0.0", "--beta", ""},
	} {
		folder := filepath.Join(dir, entry.folder)
		fakeCLI(t, filepath.Join(folder, "node_modules/.bin/linty"), "case \"$*\" in\n--version) echo "+entry.version+" ;;\n-h) printf 'Usage: linty [OPTIONS]\\n  "+entry.flag+" FILE  Required value\\n' ;;\n*) exit 23 ;;\nesac\n")
		put(t, filepath.Join(folder, "README.md"), "```sh\nnpx linty "+entry.flag+" "+entry.value+"\n```\n")
	}
	code, output, stderr := run("scan", dir, "--format=json")
	var report check.Report
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatal(err)
	}
	if code != 1 || stderr != "" || report.Summary.Errors != 1 || report.Summary.Warnings != 0 || len(report.Results) != 2 || report.Results[0].Version != "1.0.0" || report.Results[1].Version != "2.0.0" {
		t.Fatalf("project-local specs were conflated: %d %s %s", code, output, stderr)
	}
}

func TestMissingDependencyWarnsWithoutRunningNpx(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	marker := filepath.Join(dir, "WRAPPER")
	t.Setenv("POLICEDOC_TEST_MARKER", marker)
	fakeCLI(t, filepath.Join(dir, "bin/npx"), `printf 'invoked' > "$POLICEDOC_TEST_MARKER"; exit 24`)
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "```sh\nnpx policedoc-missing-test-cli .\n```\n")
	code, output, stderr := run("scan", doc, "--fail-on=warning", "--format=json")
	if code != 1 || stderr != "" || !strings.Contains(output, "help-unavailable") || !strings.Contains(output, "uncheckable") {
		t.Fatalf("missing dependency was silently accepted: %d %s %s", code, output, stderr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("npx was invoked: %v", err)
	}
}

func TestBrokenLocalDependencyDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	marker := filepath.Join(dir, "GLOBAL")
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	t.Setenv("POLICEDOC_TEST_MARKER", marker)
	fakeCLI(t, filepath.Join(dir, "bin/linty"), `printf 'invoked' > "$POLICEDOC_TEST_MARKER"; exit 25`)
	put(t, filepath.Join(dir, "node_modules/.bin/linty"), "not executable")
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "```sh\nnpx linty .\n```\n")
	code, output, _ := run("scan", doc, "--fail-on=warning", "--format=json")
	if code != 1 || !strings.Contains(output, "help-unavailable") {
		t.Fatalf("broken local dependency was not reported: %d %s", code, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("used a different global installation: %v", err)
	}
}

func TestDiscoveryCancellation(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	binary := filepath.Join(dir, "slow-cli")
	fakeCLI(t, binary, "exec /bin/sleep 10")
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "```sh\n"+binary+" status\n```\n")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if code := Run(ctx, []string{"scan", doc}, &stdout, &stderr, "test"); code != 2 || !strings.Contains(stderr.String(), "deadline exceeded") {
		t.Fatalf("cancellation ignored: %d %s", code, stderr.String())
	}
}
