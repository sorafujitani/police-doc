package app

import (
	"path/filepath"
	"testing"
)

func TestScanTranscriptsAndBuiltins(t *testing.T) {
	doc, log := lazyHelpFixture(t)
	for _, name := range []string{"cd", "✓", "Processing", "not-a-command"} {
		fakeCLI(t, filepath.Join(filepath.Dir(doc), "bin", name), "echo unexpected >> \"$POLICEDOC_TEST_CALLS\"; exit 9\n")
	}
	put(t, doc, "```sh\ncd rfmt && acme export --output result.txt\n```\n\n"+
		"```bash\n$ acme export \\\n  --output result.txt\nProcessing 25 file(s)...\n✓ 3 files formatted\n  (3 formatted, 22 unchanged)\n> not-a-command\n$ acme export --output\n```\n")
	report := scanReport(t, doc)
	if report.Summary.Examples != 4 || report.Summary.Errors != 1 || report.Summary.Warnings != 0 || report.Summary.Uncheckable != 1 {
		t.Fatalf("output or builtin affected the verdict: %+v", report)
	}
	builtin := report.Results[0]
	if len(builtin.Diagnostics) != 1 || builtin.Diagnostics[0].Code != "shell-builtin" || builtin.Diagnostics[0].Severity != "info" {
		t.Fatalf("builtin is not an informational skip: %+v", builtin)
	}
	assertHelpCalls(t, log, "--version\n-h\nexport -h\n")
}
