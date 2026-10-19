package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestBuiltinMetadataDoesNotExecuteStartupFilesOrExamples(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	put(t, "startup.sh", "printf called > startup-ran\n")
	t.Setenv("BASH_ENV", filepath.Join(dir, "startup.sh"))
	t.Setenv("ENV", filepath.Join(dir, "startup.sh"))
	t.Setenv("BASH_FUNC_builtin%%", "() { printf called > imported-function-ran; }")
	// Use a real Bash binary but never call the commands in the document.
	if err := os.Symlink(bash, filepath.Join(dir, "bash")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	put(t, "README.md", "```bash\necho -n \"not executed\" > document-command-ran\ncd /missing-directory-that-is-not-visited\npwd\n```\n")
	// Bash versions expose different synopsis grammars; assert isolation, not completeness.
	code, output, stderr := run("scan", "README.md", "--export-spec", "builtins.json")
	if code != 0 || stderr != "" {
		t.Fatalf("builtin metadata failed: %d %s %s", code, output, stderr)
	}
	for _, marker := range []string{"startup-ran", "imported-function-ran", "document-command-ran"} {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("forbidden evaluation: %s", marker)
		}
	}
	refs, err := spec.LoadReferences([]string{"builtins.json"})
	if err != nil || refs[spec.TargetKey("echo", "bash")] == nil || refs[spec.TargetKey("cd", "bash")] == nil || refs["echo"] != nil {
		t.Fatalf("builtin identity lost: %+v %v", refs, err)
	}
	t.Setenv("PATH", filepath.Join(dir, "absent"))
	if code, output, stderr := run("scan", "README.md", "--spec", "builtins.json"); code != 0 || stderr != "" {
		t.Fatalf("offline builtin rules failed: %d %s %s", code, output, stderr)
	}
}

func TestBuiltinReferenceDoesNotValidateExternalNamesakes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir)
	put(t, "builtin.json", `{"schema_version":1,"tools":[{"tool":"echo","shell":"bash","version":"test","sources":[{"kind":"reference","reference":"test contract"}],"root":{"literal_arguments":true,"forms":[{"min_args":0}]}}]}`)
	for _, cmd := range []string{"sudo echo hi", "/bin/echo hi", "bash:echo hi"} {
		put(t, "README.md", "```sh\n"+cmd+"\n```\n")
		if code, output, stderr := run("scan", "README.md", "--spec", "builtin.json", "--require-complete"); code != 1 || stderr != "" {
			t.Fatalf("builtin rules leaked to executable: %d %s %s", code, output, stderr)
		}
	}
}

func TestUnknownExportedUsageDoesNotBecomeComplete(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir)
	fakeCLI(t, filepath.Join(dir, "acme"), `case "$1" in
 --version) printf 'acme 1.0.0\n';;
 -h|--help) printf 'Usage: acme [arguments]\n';;
 *) exit 9;;
esac`)
	put(t, "README.md", "```sh\nacme file\n```\n")
	if code, _, stderr := run("scan", "README.md", "--export-spec", "ref.json"); code != 0 || stderr != "" {
		t.Fatal("failed to export unknown usage")
	}
	t.Setenv("PATH", filepath.Join(dir, "absent"))
	if code, output, stderr := run("scan", "README.md", "--spec", "ref.json", "--require-complete"); code != 1 || stderr != "" {
		t.Fatalf("export invented grammar: %d %s %s", code, output, stderr)
	}
}
