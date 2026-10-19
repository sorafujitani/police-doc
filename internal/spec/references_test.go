package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const portableReference = `{"schema_version":1,"tools":[{"tool":"foreign","version":"3.0","os":"windows","sources":[{"kind":"reference","reference":"https://example.invalid/foreign/3.0"}],"root":{"closed":true,"forms":[{"min_args":1,"max_args":2}],"flags":[{"name":"--output","aliases":["-o"],"value":"required"}]}}]}`

func TestLoadPortableReferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")
	if err := os.WriteFile(path, []byte(portableReference), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{dir, path} {
		refs, err := LoadReferences([]string{input})
		if err != nil {
			t.Fatal(err)
		}
		foreign := refs["foreign"]
		if foreign == nil || foreign.OS != "windows" || foreign.Version != "3.0" || foreign.Root.Flag("-o").Name != "--output" || len(foreign.Root.Sources) != 1 {
			t.Fatalf("lost target grammar or provenance: %+v", foreign)
		}
	}
	if _, err := LoadReferences([]string{path, path}); err == nil {
		t.Fatal("conflicting target selection was accepted")
	}
	if _, err := LoadReferences([]string{t.TempDir()}); err == nil {
		t.Fatal("empty reference directory was silently accepted")
	}
}

func TestReferenceValidationRejectsUnsupportedClaims(t *testing.T) {
	for _, tc := range []struct{ name, from, to string }{
		{"schema", `"schema_version":1`, `"schema_version":2`},
		{"unknown-field", `"closed":true`, `"closed":true,"allow_everything":true`},
		{"version", `"version":"3.0"`, `"version":"latest"`},
		{"no-evidence", `"sources":[{"kind":"reference","reference":"https://example.invalid/foreign/3.0"}]`, `"sources":[]`},
		{"min", `"min_args":1`, `"min_args":-1`},
		{"max", `"max_args":2`, `"max_args":0`},
		{"unknown-form-flag", `"min_args":1`, `"min_args":1,"required_flags":["--missing"]`},
		{"alias-conflict", `"aliases":["-o"]`, `"aliases":["--output"]`},
		{"form-alias-conflict", `"min_args":1`, `"min_args":1,"required_flags":["-o"],"forbidden_flags":["--output"]`},
		{"invalid-forwarding", `"value":"required"`, `"value":"none","forward":true`},
		{"invalid-terminator", `"value":"required"`, `"value":"required","terminator":";"`},
		{"flag-order", `"closed":true`, `"closed":true,"flags_after_positionals":true,"stop_flags_after_operand":true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reference.json")
			if err := os.WriteFile(path, []byte(strings.Replace(portableReference, tc.from, tc.to, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadReferences([]string{path}); err == nil {
				t.Fatal("invalid specification was accepted")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "multiple.json")
	if err := os.WriteFile(path, []byte(portableReference+"\n{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReferences([]string{path}); err == nil {
		t.Fatal("trailing JSON was ignored")
	}
}
