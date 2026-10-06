package spec

import (
	"reflect"
	"testing"
)

// Untrusted help text may yield partial metadata, never invalid or complete grammar.
func FuzzParseHelp(f *testing.F) {
	for _, help := range []string{
		"",
		"Usage: acme [-o FILE]\nOptions:\n  -o, --output FILE  Output\n",
		"Commands:\n  status  Show status\n  publish  Publish\n",
		"Options:\n  --[no-]color[=WHEN]\n  --output FILE\n  --output[=FILE]\n",
		"\x1b[32m--verbose\x1b[0m\n",
		"Usage: acme run nested [OPTIONS]\n  --output FILE\n",
		"Example usage:\n  acme other\nOptions:\n  --output FILE\n",
		"Usage: /runtime/Canonical COMMAND\nCommands:\n  run  Run\n",
	} {
		f.Add(help)
	}
	f.Fuzz(func(t *testing.T, help string) {
		if len(help) > 16384 {
			t.Skip()
		}
		command := parseHelp(help)
		snapshot := Snapshot{Tool: "acme", Version: "1.0.0", Root: command}
		if err := snapshot.Validate(); err != nil {
			t.Fatalf("help %q produced an invalid snapshot: %v; %+v", help, err, command)
		}
		if !reflect.DeepEqual(command, parseHelp(help)) {
			t.Fatal("help parsing is nondeterministic")
		}
		for _, path := range [][]string{nil, {"run"}} {
			if parsed, err := parseHelpPage(help, "acme", "acme", path); err == nil {
				snapshot.Root = parsed
				if err := snapshot.Validate(); err != nil {
					t.Fatalf("scoped help %q produced an invalid snapshot: %v", help, err)
				}
			}
		}
	})
}
