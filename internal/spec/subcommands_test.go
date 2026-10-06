package spec

import "testing"

func TestCommandSectionsWithoutColons(t *testing.T) {
	command, err := parseHelpPage(`USAGE
  acme <command> [flags]

CORE COMMANDS
  auth:          Authenticate
  browse:        Browse

WORKFLOW COMMANDS
  run:           Run a workflow

EXTENSION COMMANDS
  local-tool:    Installed extension

ADDITIONAL COMMANDS
  alias:         Manage aliases

HELP TOPICS
  formatting:    Not a command

EXAMPLES
  bogus:         Not a command

FLAGS
  --help         Show help
`, "acme", "acme", nil)
	if err != nil || !command.SubcommandFirst || command.UsageName != "acme" {
		t.Fatalf("command usage was not recognized: %+v, %v", command, err)
	}
	if len(command.Commands) != 5 {
		t.Fatalf("wrong command list: %+v", command.Commands)
	}
	for _, name := range []string{"auth", "browse", "run", "local-tool", "alias"} {
		if command.Child(name) == nil {
			t.Errorf("missing command %q", name)
		}
	}
}

func TestSubcommandPositionRequiresUnambiguousUsage(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		want        bool
	}{
		{"required", "Usage: acme <command> [flags]", true},
		{"optional", "Usage: acme [command]", true},
		{"flags-first", "Usage: acme [flags] COMMAND", true},
		{"options-first", "Usage: acme [OPTIONS] <subcommand>", true},
		{"inline-option", "Usage: acme [-C <path>] <command>", true},
		{"nested-option", "Usage: acme [--color[=WHEN]] <command>", true},
		{"positional-first", "Usage: acme FILE <command>", false},
		{"optional-positional-first", "Usage: acme [FILE] <command>", false},
		{"positional-only", "Usage: acme FILE", false},
		{"option-or-positional", "Usage: acme [-v | FILE] <command>", false},
		{"command-or-file", "Usage: acme [COMMAND|FILE]", false},
		{"multiple-command-synopses", "Usage: acme COMMAND\n       acme [flags] COMMAND", true},
		{"mixed-synopses", "Usage: acme COMMAND\n       acme FILE", false},
		{"mixed-headings", "Usage: acme COMMAND\n\nUsage: acme FILE", false},
		{"no-usage", "Acme help", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command, err := parseHelpPage(tc.usage+"\n\nCommands:\n  run  Run something\n", "acme", "acme", nil)
			if err != nil || command.SubcommandFirst != tc.want {
				t.Fatalf("SubcommandFirst=%v, %v; want %v", command.SubcommandFirst, err, tc.want)
			}
		})
	}
}

func TestColonlessUsageStillRejectsWrongHelp(t *testing.T) {
	for _, synopsis := range []string{"acme <command>", "acme auth nested [flags]"} {
		if _, err := parseHelpPage("USAGE\n  "+synopsis+"\n\nFLAGS\n  --token FILE\n", "acme", "acme", []string{"auth"}); err == nil {
			t.Fatalf("accepted unrelated usage: %s", synopsis)
		}
	}
	command, err := parseHelpPage("USAGE\n  acme auth login\n", "acme", "acme", []string{"auth", "login"})
	if err != nil || command.SubcommandFirst {
		t.Fatalf("usage-only leaf rejected: %+v, %v", command, err)
	}
}
