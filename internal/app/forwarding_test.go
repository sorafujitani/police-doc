package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
)

func TestEmptyForwardingDoesNotHideParentTail(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir)
	put(t, "reference.json", referenceFixture)
	for _, command := range []string{`launcher --exec \; $ARGS`, `launcher --exec '' \; $ARGS`, `launcher --exec= \; $ARGS`} {
		put(t, "README.md", "```sh\n"+command+"\n```\n")
		code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--require-complete", "--fail-on=none", "--format=json")
		var report check.Report
		if err := json.Unmarshal([]byte(output), &report); err != nil || code != 1 || stderr != "" || report.CoverageComplete || !strings.Contains(output, "dynamic-argument") || !strings.Contains(output, "missing-flag-value") {
			t.Fatalf("an invalid forward hid its unresolved parent tail: %d %s %s %v", code, output, stderr, err)
		}
	}
}

func TestAttachedForwardingTerminatorMatchesSeparateForm(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir)
	put(t, "reference.json", strings.Replace(referenceFixture, `"terminator": ";"`, `"terminator": "pack"`, 1))
	for _, command := range []string{"launcher --exec pack add core", "launcher --exec=pack add core"} {
		put(t, "README.md", "```sh\n"+command+"\n```\n")
		code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--require-complete", "--format=json")
		if code != 1 || stderr != "" || !strings.Contains(output, "missing-flag-value") {
			t.Fatalf("attached terminator became a valid child: %d %s %s", code, output, stderr)
		}
	}
}

func TestForwardedWrappersAreNeverExecuted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir)
	put(t, "reference.json", referenceFixture)
	for _, wrapper := range []string{"sudo", "npx"} {
		fakeCLI(t, filepath.Join(dir, wrapper), "printf called >> wrapper-calls\nexit 9\n")
	}
	fakeCLI(t, filepath.Join(dir, "inner"), `
printf called >> inner-calls
case "$1" in
  --version) printf 'inner 1.0.0\n' ;;
  -h|--help) printf 'Usage: inner [options]\nOptions:\n  --quiet  Be quiet\n' ;;
  *) printf called > document-command-ran; exit 9 ;;
esac
`)
	for _, wrapper := range []string{"sudo -n", "npx --no-install"} {
		put(t, "README.md", "```sh\nlauncher --exec "+wrapper+" pack add core\n```\n")
		if code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--require-complete"); code != 0 || stderr != "" {
			t.Fatalf("forwarded wrapper was not resolved: %d %s %s", code, output, stderr)
		}
		put(t, "README.md", "```sh\nlauncher --exec "+wrapper+" inner --quiet\n```\n")
		code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--allow-live-help", "--require-complete", "--format=json")
		if code != 0 || stderr != "" || strings.Contains(output, "help-unavailable") || strings.Contains(output, "missing-reference") {
			t.Fatalf("explicit live fallback did not reach the inner CLI: %d %s %s", code, output, stderr)
		}
	}
	if _, err := os.Stat("inner-calls"); err != nil {
		t.Fatal("the inner CLI's metadata was not collected")
	}
	for _, marker := range []string{"wrapper-calls", "document-command-ran"} {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("forbidden invocation occurred: %s", marker)
		}
	}
}
