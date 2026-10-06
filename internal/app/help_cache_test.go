package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/sorafujitani/police-doc/internal/check"
	"github.com/sorafujitani/police-doc/internal/spec"
)

func cacheFixture(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Join(dir, "bin"))
	log := filepath.Join(dir, "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	t.Setenv("ALPHA_VERSION", "1.0.0")
	for _, name := range []string{"alpha", "beta"} {
		version := "1.0.0"
		if name == "alpha" {
			version = "$ALPHA_VERSION"
		}
		fakeCLI(t, filepath.Join(dir, "bin", name), fmt.Sprintf(`
printf '%s %%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) printf '%%s\n' "%s" ;;
  -h) printf 'Usage: %s [OPTIONS]\n  --output FILE  Output file\n' ;;
  *) exit 26 ;;
esac
`, name, version, name))
	}
	doc := filepath.Join(dir, "README.md")
	put(t, doc, "```sh\nalpha --output first\nalpha --output second\n```\n")
	return dir, doc, log
}

func assertHelpCalls(t *testing.T, log, want string) {
	t.Helper()
	calls, err := os.ReadFile(log)
	if os.IsNotExist(err) && want == "" {
		return
	}
	if err != nil || string(calls) != want {
		t.Fatalf("calls=%q, want=%q: %v", calls, want, err)
	}
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func cacheEntries(t *testing.T, dir string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, ".policedoc/cache/*.json"))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]string)
	for _, path := range paths {
		var entry cachedHelp
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &entry); err != nil {
			t.Fatal(err)
		}
		entries[entry.Snapshot.Tool] = path
	}
	return entries
}

func TestIncrementalHelpCache(t *testing.T) {
	dir, doc, log := cacheFixture(t)
	scan := func(args ...string) check.Report {
		t.Helper()
		code, output, stderr := run(append([]string{"scan", doc, "--format=json"}, args...)...)
		var report check.Report
		if err := json.Unmarshal([]byte(output), &report); err != nil || code != 0 || stderr != "" || report.Summary.Warnings != 0 {
			t.Fatalf("scan failed: %d %s %s %v", code, output, stderr, err)
		}
		return report
	}
	scan()
	assertHelpCalls(t, log, "alpha --version\nalpha -h\n")
	alphaCache := cacheEntries(t, dir)["alpha"]
	before, err := os.Stat(alphaCache)
	if err != nil {
		t.Fatal(err)
	}
	put(t, doc, "```sh\nalpha --output changed-document\n```\n")
	scan()
	assertHelpCalls(t, log, "alpha --version\n")
	after, err := os.Stat(alphaCache)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("unchanged specification was rewritten")
	}
	put(t, doc, "```sh\nalpha --output one\nbeta --output two\n```\n")
	scan()
	assertHelpCalls(t, log, "alpha --version\nbeta --version\nbeta -h\n")
	t.Setenv("ALPHA_VERSION", "2.0.0") // Version changes without touching the executable.
	report := scan()
	assertHelpCalls(t, log, "alpha --version\nalpha -h\nbeta --version\n")
	if report.Results[0].Version != "2.0.0" || report.Results[1].Version != "1.0.0" {
		t.Fatal("wrong versions after incremental refresh")
	}
	binary := filepath.Join(dir, "bin/alpha")
	file, err := os.OpenFile(binary, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n# executable changed without a version bump\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	scan()
	assertHelpCalls(t, log, "alpha --version\nalpha -h\nbeta --version\n")
	scan("--refresh")
	assertHelpCalls(t, log, "alpha --version\nalpha -h\nbeta --version\nbeta -h\n")
}

func TestHelpCacheRecovery(t *testing.T) {
	dir, doc, log := cacheFixture(t)
	if code, _, stderr := run("scan", doc); code != 0 || stderr != "" {
		t.Fatalf("initial scan failed: %d %s", code, stderr)
	}
	assertHelpCalls(t, log, "alpha --version\nalpha -h\n")
	path := cacheEntries(t, dir)["alpha"]
	binary := filepath.Join(dir, "bin/alpha")
	if err := os.Rename(binary, binary+".saved"); err != nil {
		t.Fatal(err)
	}
	code, output, stderr := run("scan", doc, "--format=json", "--fail-on=warning")
	if code != 1 || stderr != "" || !strings.Contains(output, "help-unavailable") || strings.Contains(output, `"version": "1.0.0"`) {
		t.Fatalf("cached help used without an installed CLI: %d %s %s", code, output, stderr)
	}
	assertHelpCalls(t, log, "")
	if err := os.Rename(binary+".saved", binary); err != nil {
		t.Fatal(err)
	}
	put(t, path, "{broken JSON")
	if code, output, stderr := run("scan", doc); code != 0 || stderr != "" || strings.Contains(output, "help-unavailable") {
		t.Fatalf("corrupt cache was not rebuilt: %d %s %s", code, output, stderr)
	}
	assertHelpCalls(t, log, "alpha --version\nalpha -h\n")
	if _, err := loadHelpCache(path, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	code, output, stderr = run("scan", doc, "--fail-on=warning")
	if code != 0 || stderr != "" || strings.Contains(output, "help-unavailable") {
		t.Fatalf("missing cache was not rebuilt: %d %s %s", code, output, stderr)
	}
	assertHelpCalls(t, log, "alpha --version\nalpha -h\n")
}

func TestChangedCLIRejectsStaleCache(t *testing.T) {
	dir, doc, log := cacheFixture(t)
	if code, _, _ := run("scan", doc); code != 0 {
		t.Fatal("initial scan failed")
	}
	assertHelpCalls(t, log, "alpha --version\nalpha -h\n")
	path := cacheEntries(t, dir)["alpha"]
	fakeCLI(t, filepath.Join(dir, "bin/alpha"), `
printf 'alpha %s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 2.0.0 ;;
  *) echo 'error: unavailable help'; exit 2 ;;
esac
`)
	code, output, _ := run("scan", doc, "--fail-on=warning", "--format=json")
	if code != 1 || !strings.Contains(output, "help-unavailable") || strings.Contains(output, `"version": "1.0.0"`) {
		t.Fatalf("stale specification was used online: %d %s", code, output)
	}
	assertHelpCalls(t, log, "alpha --version\nalpha -h\nalpha --help\n")
	previous, err := loadHelpCache(path, "alpha")
	if err != nil || previous.Snapshot.Version != "1.0.0" {
		t.Fatalf("failed collection overwrote the last usable cache: %+v %v", previous, err)
	}
}

func TestCacheWriteFailureDoesNotDiscardCollectedHelp(t *testing.T) {
	dir, doc, log := cacheFixture(t)
	path := filepath.Join(dir, "not-a-directory")
	put(t, path, "keep this file")
	code, output, stderr := run("scan", doc, "--cache-dir", path, "--format=json", "--fail-on=warning")
	if code != 1 || stderr != "" || !strings.Contains(output, "Could not save help cache") || !strings.Contains(output, `"version": "1.0.0"`) {
		t.Fatalf("cache write failure was mishandled: %d %s %s", code, output, stderr)
	}
	assertHelpCalls(t, log, "alpha --version\nalpha -h\n")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep this file" {
		t.Fatal("existing file changed")
	}
}

func TestHelpCacheAtomicReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	entry := func(version string) cachedHelp {
		return cachedHelp{SchemaVersion: helpCacheVersion, Binary: "alpha", Fingerprint: "fingerprint", Snapshot: &spec.Snapshot{
			Tool: "alpha", Version: version, OS: runtime.GOOS,
		}}
	}
	if err := writeHelpCache(path, entry("1.0.0")); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for _, version := range []string{"1.0.0", "2.0.0"} {
		group.Go(func() {
			for range 10 {
				if err := writeHelpCache(path, entry(version)); err != nil {
					t.Error(err)
				}
				if _, err := loadHelpCache(path, "alpha"); err != nil {
					t.Errorf("reader observed an incomplete cache: %v", err)
				}
			}
		})
	}
	group.Wait()
}

func TestHelpCacheRejectsUnsupportedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	entry := cachedHelp{SchemaVersion: helpCacheVersion, Binary: "alpha", Fingerprint: "fingerprint",
		Snapshot: &spec.Snapshot{Tool: "alpha", Version: "1.0.0", OS: runtime.GOOS}}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(data)
	put(t, path, valid)
	if _, err := loadHelpCache(path, "alpha"); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		"{broken", "{}", valid + "{}",
		strings.Replace(valid, fmt.Sprintf(`"schema_version":%d`, helpCacheVersion), fmt.Sprintf(`"schema_version":%d`, helpCacheVersion-1), 1),
		strings.Replace(valid, `"root":{}`, `"root":{"coverage":{"flags":true}}`, 1),
		strings.Replace(valid, `"tool":"alpha"`, `"tool":"other"`, 1),
		valid + strings.Repeat(" ", (8<<20)+1),
	} {
		put(t, path, data)
		if _, err := loadHelpCache(path, "alpha"); err == nil {
			t.Fatalf("unsupported cache accepted (length %d)", len(data))
		}
	}
}
