package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sorafujitani/police-doc/internal/spec"
)

func TestExportedSynopsisRulesWorkOffline(t *testing.T) {
	doc := scanFixture(t, "acme status --output file")
	output := filepath.Join(t.TempDir(), "spec.json")
	if code, stdout, stderr := run("scan", doc, "--export-spec", output); code != 0 || stderr != "" {
		t.Fatalf("export failed: %d %s %s", code, stdout, stderr)
	}
	references, err := spec.LoadReferences([]string{output})
	if err != nil {
		t.Fatal(err)
	}
	command := references["acme"].Root.Child("status")
	if command == nil || len(command.Forms) == 0 || !command.DerivedForms || command.Closed {
		t.Fatal("export lost the distinction between a synopsis and a closed reference")
	}
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))
	if code, _, stderr := run("scan", doc, "--spec", output, "--require-complete"); code != 0 || stderr != "" {
		t.Fatalf("exported synopsis did not retain its argument rules: %d %s", code, stderr)
	}
	second := filepath.Join(t.TempDir(), "second.json")
	if code, _, stderr := run("scan", doc, "--spec", output, "--export-spec", second); code != 0 || stderr != "" {
		t.Fatalf("offline round trip failed: %d %s", code, stderr)
	}
	reloaded, err := spec.LoadReferences([]string{second})
	if err != nil || !reflect.DeepEqual(references, reloaded) {
		t.Fatalf("export lost reference facts: %v", err)
	}
}

func TestExportCannotOverwriteMarkdownOrInventMissingSpecs(t *testing.T) {
	doc := scanFixture(t, "acme status")
	before, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("scan", doc, "--export-spec", doc); code != 2 {
		t.Fatal("export accepted a Markdown output path")
	}
	after, err := os.ReadFile(doc)
	if err != nil || string(after) != string(before) {
		t.Fatal("export modified its input")
	}
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))
	output := filepath.Join(t.TempDir(), "missing.json")
	if code, _, _ := run("scan", doc, "--export-spec", output); code != 2 {
		t.Fatal("export accepted missing metadata")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed export created an unusable reference")
	}
}
