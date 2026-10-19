package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
)

func TestPinnedDNF5ReferenceChecksRealOSSExamplesOffline(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "specs", "dnf5.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "no-installed-clis"))
	put(t, "dnf5.json", string(data))
	// Corpus snippets plus the documented zero-operand upgrade form.
	for _, tc := range []struct {
		command string
		valid   bool
		finding string
	}{
		{"sudo dnf install ripgrep", true, ""},
		{"dnf install fd-find", true, ""},
		{"sudo dnf install dnf5-plugins", true, ""},
		{"sudo dnf update gh", true, ""},
		{"dnf upgrade", true, ""},
		{"sudo dnf config-manager addrepo --from-repofile=https://cli.github.com/packages/rpm/gh-cli.repo", true, ""},
		// Negative controls change the source examples rather than only testing
		// known-good text against permissive specifications.
		{"sudo dnf install", false, "invalid-arguments"},
		{"dnf install fd-find --not-a-documented-flag", false, "unverified-flag"},
		{"sudo dnf config-manager addrepo --from-repofile", false, "missing-flag-value"},
		{"sudo dnf config-manager addrepo --from-repofile=gh.repo extra", false, "invalid-arguments"},
		{"sudo dnf config-manager --add-repo gh.repo", false, "unverified-flag"},
	} {
		put(t, "README.md", "```sh\n"+tc.command+"\n```\n")
		code, output, stderr := run("scan", "README.md", "--spec", "dnf5.json", "--require-complete", "--format=json")
		var report check.Report
		if err := json.Unmarshal([]byte(output), &report); err != nil || stderr != "" || (code == 0) != tc.valid {
			t.Fatalf("%s: %d %s %s %v", tc.command, code, output, stderr, err)
		}
		if tc.valid && (!report.CoverageComplete || report.Summary.Checked != 1 || report.Results[0].OS != "linux" || len(report.Results[0].Diagnostics) != 0) {
			t.Fatalf("foreign CLI was not fully checked against its reference: %s", output)
		}
		if tc.finding != "" && !strings.Contains(output, tc.finding) {
			t.Fatalf("mutation did not produce %s: %s", tc.finding, output)
		}
	}
}
