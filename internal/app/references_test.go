package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
)

const referenceFixture = `{
  "schema_version": 1,
  "tools": [
    {
      "tool": "pack", "version": "2.0", "os": "linux",
      "sources": [{"kind": "reference", "reference": "https://example.invalid/pack/2.0/manual"}],
      "root": {
        "closed": true, "subcommand_first": true,
        "commands": [{
          "name": "add", "aliases": ["install"], "closed": true,
          "flags_after_positionals": true, "forms": [{"min_args": 1}],
          "flags": [{"name": "--target", "aliases": ["-t"], "value": "required"}]
        }]
      }
    },
    {
      "tool": "launcher", "version": "1.0",
      "sources": [{"kind": "reference", "reference": "https://example.invalid/launcher/1.0/manual"}],
      "root": {
        "closed": true, "forms": [{"min_args": 0, "max_args": 0}],
        "flags": [
          {"name": "--exec", "value": "required", "forward": true, "terminator": ";"},
          {"name": "--timeout", "value": "required"}
        ]
      }
    }
  ]
}`

func TestReferencesWithoutInstalledExecutables(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "no-executables"))
	put(t, "reference.json", referenceFixture)
	for _, tc := range []struct {
		command string
		code    int
		checked bool
		finding string
	}{
		{"pack add core", 0, true, ""},
		{"pack install core --target cache", 0, true, ""},
		{`pack add core >"$(unverified --bad)"`, 1, false, "unchecked-substitution"},
		{"pack add --target cache", 1, true, "invalid-arguments"},
		{"pack add core --target", 1, true, "missing-flag-value"},
		{"pack add core --typo", 1, false, "unknown-flag"},
		{`launcher --exec pack add core \; --timeout 3`, 0, true, ""},
		{`launcher --exec pack add \; --timeout 3`, 1, true, "invalid-arguments"},
		{"launcher --exec missing-command --arg", 1, false, "missing-reference"},
		{`launcher --exec pack add -- "$SEP" core`, 1, false, "dynamic-forwarding-boundary"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			put(t, "README.md", "```sh\n"+tc.command+"\n```\n")
			code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--require-complete", "--format=json")
			var report check.Report
			if err := json.Unmarshal([]byte(output), &report); err != nil || code != tc.code || stderr != "" || report.CoverageComplete != tc.checked {
				t.Fatalf("scan: %d %s %s %v", code, output, stderr, err)
			}
			if report.SchemaVersion != 3 || len(report.Results) != 1 || report.Results[0].SpecSource != "reference" {
				t.Fatalf("missing reference provenance: %s", output)
			}
			if tc.finding != "" && !strings.Contains(output, tc.finding) {
				t.Fatalf("missing %s: %s", tc.finding, output)
			}
			if tc.checked && report.Summary.Checked != 1 {
				t.Fatalf("checked count lost: %s", output)
			}
		})
	}
}

func TestReferenceSelectionNeverExecutesTheTarget(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir)
	fakeCLI(t, filepath.Join(dir, "pack"), "printf called > calls\nexit 9\n")
	put(t, "reference.json", referenceFixture)
	put(t, "README.md", "```sh\npack add core\n```\n")
	code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--require-complete")
	if code != 0 || stderr != "" {
		t.Fatalf("reference-only scan failed: %d %s %s", code, output, stderr)
	}
	if _, err := os.Stat("calls"); !os.IsNotExist(err) {
		t.Fatal("reference selection executed the target")
	}
	fakeCLI(t, filepath.Join(dir, "other"), "printf called > calls\nexit 9\n")
	for _, command := range []string{"other", "launcher --exec other"} {
		put(t, "README.md", "```sh\n"+command+"\n```\n")
		code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--require-complete")
		if code != 1 || stderr != "" || !strings.Contains(output, "missing-reference") {
			t.Fatalf("missing reference was not reported: %d %s %s", code, output, stderr)
		}
		if _, err := os.Stat("calls"); !os.IsNotExist(err) {
			t.Fatal("reference-only mode executed an unlisted target")
		}
	}
	put(t, "bad.json", strings.Replace(referenceFixture, `"min_args": 1`, `"min_args": -1`, 1))
	if code, _, _ := run("scan", "README.md", "--spec", "bad.json"); code != 2 {
		t.Fatal("invalid reference silently fell back to executing a CLI")
	}
	if _, err := os.Stat("calls"); !os.IsNotExist(err) {
		t.Fatal("invalid reference executed the target")
	}
	if code, _, stderr := run("scan", "README.md", "--spec", "reference.json", "--allow-live-help", "--require-complete"); code != 1 || stderr != "" {
		t.Fatalf("explicit live fallback failed unexpectedly: %d %s", code, stderr)
	}
	if _, err := os.Stat("calls"); err != nil {
		t.Fatal("explicit live fallback did not collect metadata")
	}
}

func TestCompleteCoverageCannotBeBypassedByFindingThreshold(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "empty"))
	for _, body := range []string{"no CLI examples", "```sh\nmissing --flag\n```\n", "```sh\n$DYNAMIC --flag\n```\n"} {
		put(t, "README.md", body)
		code, output, stderr := run("scan", "README.md", "--fail-on=none", "--require-complete")
		if code != 1 || stderr != "" || !strings.Contains(output, "Incomplete coverage") {
			t.Fatalf("incomplete scan passed: %d %s %s", code, output, stderr)
		}
	}
}

func TestForwardingDepthIsBounded(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "empty"))
	put(t, "reference.json", referenceFixture)
	put(t, "README.md", "```sh\n"+strings.Repeat("launcher --exec ", 12)+"pack add core\n```\n")
	code, output, stderr := run("scan", "README.md", "--spec", "reference.json", "--require-complete", "--format=json")
	if code != 1 || stderr != "" || !strings.Contains(output, "forwarding-limit") {
		t.Fatalf("unbounded forwarding: %d %s %s", code, output, stderr)
	}
}
