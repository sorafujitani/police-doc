package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferencesRejectAmbiguousAliases(t *testing.T) {
	for _, commands := range []string{
		`[{"name":"one","aliases":["x"]},{"name":"two","aliases":["x"]}]`,
		`[{"name":"one","aliases":["two"]},{"name":"two"}]`,
		`[{"name":"one","aliases":["one"]}]`,
	} {
		path := filepath.Join(t.TempDir(), "reference.json")
		data := strings.Replace(portableReference, `"root":{`, `"root":{"commands":`+commands+`,`, 1)
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadReferences([]string{path}); err == nil || !strings.Contains(err.Error(), "conflicting command or alias") {
			t.Fatalf("ambiguous references were not rejected: %v", err)
		}
	}
}

func TestReferenceNestingIsBounded(t *testing.T) {
	var reference References
	if err := json.Unmarshal([]byte(portableReference), &reference); err != nil {
		t.Fatal(err)
	}
	child := Command{Name: "leaf"}
	for range 66 {
		child = Command{Name: "sub", Commands: []Command{child}}
	}
	reference.Tools[0].Root.Commands = []Command{child}
	data, err := json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deep.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReferences([]string{path}); err == nil || !strings.Contains(err.Error(), "64 levels") {
		t.Fatalf("unbounded nesting was not rejected: %v", err)
	}
}
