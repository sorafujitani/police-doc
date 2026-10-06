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
		"\x1b]8;;https://example.test/--fake\x1b\\--verbose\x1b]8;;\x1b\\\n",
		"\x1bP\nOptions:\n  --fake\n\x1b\\Usage: acme\n",
		"\x1b]unterminated",
		"\u009b32m日本語\u009b0m",
		"Usage: acme run nested [OPTIONS]\n  --output FILE\n",
		"Example usage:\n  acme other\nOptions:\n  --output FILE\n",
		"Usage: /runtime/Canonical COMMAND\nCommands:\n  run  Run\n",
		"USAGE\n  acme <command> [flags]\n\nCORE COMMANDS\n  run:  Run\n\nHELP TOPICS\n  format:  Format help\n",
		"Usage: acme COMMAND\n       acme FILE\nCommands:\n  run  Run\n",
		"Usage: acme [-v | --version]\n       <command> [<args>]\nCommands:\n  run  Run\n",
		"Usage: acme run [NUMBER] [flags]\n\nAliases:\n  acme r\n\nFlags:\n  -s, --state string  State: {open|closed}\n  -d, --draft\n",
		"Usage: acme [OPTIONS]\n\nOPTIONS\n     --color[=WHEN]\n         --color=always. Prose\n\nEXAMPLES\n  --not-a-flag FILE\n",
		"Options:\n  --state {a,b}\n  --state {b,a}\n  --template string  Template (default: {a|b})\n",
	} {
		f.Add(help)
	}
	f.Fuzz(func(t *testing.T, help string) {
		if len(help) > 16384 {
			t.Skip()
		}
		plain, err := normalizeTerminalOutput(help)
		if err != nil {
			return
		}
		if again, err := normalizeTerminalOutput(plain); err != nil || again != plain {
			t.Fatalf("normalization is not idempotent: %q -> %q (%v)", help, plain, err)
		}
		help = plain
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
