package spec

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeTerminalOutput(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"plain-layout", "Usage: acme\r\n\t--名前 FILE  café 中文 🐈\n", "Usage: acme\r\n\t--名前 FILE  café 中文 🐈\n"},
		{"sgr", "\x1b[1;38;2;10;20;30m--flag\x1b[0m", "--flag"},
		{"csi-private-and-intermediate", "\x1b[?25l\x1b[1 qUsage: acme\x1b[?25h", "Usage: acme"},
		{"osc-st", "\x1b]8;id=flag;https://example.test/--fake\x1b\\-s, --symbolic\x1b]8;;\x1b\\", "-s, --symbolic"},
		{"osc-bel", "\x1b]8;;https://example.test/9.9.9\a1.2.3\x1b]8;;\a", "1.2.3"},
		{"mixed-decoration", "  \x1b]8;;https://example.test\x1b\\\x1b[1m--output\x1b[0m\x1b]8;;\x1b\\ FILE", "  --output FILE"},
		{"title", "\x1b]0;\nOptions:\n  --fake\aUsage: acme", "Usage: acme"},
		{"control-strings", "a\x1bP--fake\x1b\\b\x1bX--fake\x1b\\c\x1b^--fake\x1b\\d\x1b_--fake\x1b\\e", "abcde"},
		{"bel-does-not-end-dcs", "\x1bP\a--fake\x1b\\--real", "--real"},
		{"escape-inside-string", "\x1b]0;\x1b[31m--fake\x1b\\--real", "--real"},
		{"simple-escape", "\x1b(B\x1b7Usage: acme\x1b8", "Usage: acme"},
		{"utf8-c1", "\u009d8;;url\u009c\u009b1m--flag\u009b0m\u009d8;;\u009c", "--flag"},
		{"byte-c1", "\x9d8;;url\x9c\x9b1m--flag\x9b0m\x9d8;;\x9c", "--flag"},
		{"c1-control-strings", "\u0090--fake\u009c\u0098--fake\u009c\u009e--fake\u009c\u009f--fake\u009c--real", "--real"},
		{"standalone-controls", "\x00\a\x7f\u0085Usage: acme\n", "Usage: acme\n"},
		{"invalid-utf8-text", "text \xff", "text \xff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeTerminalOutput(tc.input)
			if err != nil || got != tc.want {
				t.Fatalf("normalize(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestNormalizeTerminalOutputRejectsBrokenSequences(t *testing.T) {
	for _, input := range []string{
		"\x1b", "\x1b(", "\x1b[", "\x1b[31", "\x1b[1 2m", "\x1b[☃",
		"\x1b\n", "\x1b(\n", "\x1b]8;;url", "\x1b]0;title\x1b",
		"\x1bP--fake\a", "\x1bX--fake", "\x1b^--fake", "\x1b_--fake",
		"\u009b", "\x9dtitle",
	} {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			if got, err := normalizeTerminalOutput("Usage: acme\n" + input); err == nil || got != "" {
				t.Fatalf("malformed metadata was accepted: %q, %v", got, err)
			}
		})
	}
}

func TestTerminalNormalizationPrecedesHelpStatusChecks(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		exit         int
		wantError    string
	}{
		{"nonzero-usage", "\x1b]8;;https://example.test\a\x1b[1mUsage:\x1b[0m\x1b]8;;\a acme\n", 129, ""},
		{"colored-error", "\x1b[31merror:\x1b[0m unsupported\nUsage: acme\n", 0, "CLI reported an error"},
		{"linked-error", "\x1b]8;;https://example.test\x1b\\error:\x1b]8;;\x1b\\ unsupported\nUsage: acme\n", 2, "CLI reported an error"},
		{"hidden-error", "\x1b]0;error: invisible\aUsage: acme\n", 0, ""},
		{"broken-control", "Usage: acme\n\x1b]8;;url", 0, "unterminated terminal control sequence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binary := helpExecutable(t, fmt.Sprintf("printf '%%s' '%s'; exit %d", strings.ReplaceAll(tc.output, "'", "'\\''"), tc.exit))
			got, err := commandOutput(t.Context(), binary, nil, true, "-h")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected %q, got %q, %v", tc.wantError, got, err)
				}
			} else if err != nil || !strings.Contains(got, "Usage: acme") {
				t.Fatalf("normal help was rejected: %q, %v", got, err)
			}
		})
	}
}

func TestCollectDecoratedMetadata(t *testing.T) {
	binary := helpExecutable(t, `
case "$*" in
  --version) printf '\033]8;;https://example.test/9.9.9\007\033[1macme 1.2.3\033[0m\033]8;;\007\n' ;;
  -h) printf '\033]0;\nCommands:\n  fake  Invisible\007\033[1mUsage:\033[0m acme COMMAND\nCommands:\n  \033]8;;https://example.test/run\033\\run\033]8;;\033\\  Run something\n' ;;
  'run -h') printf '\033]8;;https://example.test/usage\007Usage: acme run [OPTIONS]\033]8;;\007\nOptions:\n  \033]8;;https://example.test/--fake\033\\\033[1m-s, --symbolic\033[0m\033]8;;\033\\\n  \033[1m--output\033[0m FILE\n'; exit 129 ;;
  *) exit 11 ;;
esac
`)
	snapshot := collectTestPaths(t, HelpOptions{Binary: binary}, "run")
	if snapshot.Version != "1.2.3" || snapshot.Root.Child("fake") != nil || len(snapshot.Root.Commands) != 1 {
		t.Fatalf("control payload leaked into metadata: %+v", snapshot)
	}
	command := snapshot.Root.Child("run")
	if command == nil || command.Flag("-s") == nil || command.Flag("--symbolic") == nil || len(command.Flags) != 3 {
		t.Fatalf("linked flags were not collected: %+v", command)
	}
	if flag := command.Flag("--output"); flag == nil || flag.Value != "required" {
		t.Fatalf("flag value was lost: %+v", flag)
	}
}
