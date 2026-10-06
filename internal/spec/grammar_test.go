package spec

import (
	"slices"
	"testing"
)

func TestWrappedUsageAndTrailingFlags(t *testing.T) {
	for _, tc := range []struct {
		usage                  string
		commandFirst, trailing bool
	}{
		{"Usage: acme [-v | --version]\n           [--config FILE]\n           <command> [<args>]", true, false},
		{"USAGE\n  acme [<number> | <url> | <branch>]\n       [flags]", false, true},
		{"Usage: acme [OPTIONS] FILE", false, false},
		{"Usage: acme FILE [flags] COMMAND [ARGS...]", false, false},
		{"Usage: acme FILE [flags]\n       acme COMMAND [ARGS...]", false, false},
		{"Usage: acme [-v | FILE] <command>", false, false},
	} {
		t.Run(tc.usage, func(t *testing.T) {
			command, err := parseHelpPage(tc.usage+"\n\nCommands:\n  run  Run\n", "acme", "acme", nil)
			if err != nil || command.SubcommandFirst != tc.commandFirst || command.FlagsAfterPositionals != tc.trailing {
				t.Fatalf("unexpected usage: %+v, %v", command, err)
			}
		})
	}
}

func TestExplicitValueRulesAndChoices(t *testing.T) {
	command := parseHelp(`Usage: acme [--one {a,b}] [--two {c,d}]
Options:
  -d, --draft  Filter drafts
  -s, --state string  Filter by state: {open|closed|merged|all}
  --format {json, text}  Format
  --mode {--fast|--slow}  Mode
  --output FILE  Output
  --output
    A repeated bare heading must not erase its value requirement.
  --example string  For example, use {not|choices} here
  --broken {open|closed>  Malformed enumeration
  --conflict {a,b}  First definition
  --conflict {c,d}  Conflicting definition
`)
	for _, tc := range []struct {
		name, value string
		choices     []string
	}{
		{"-d", "none", nil}, {"--draft", "none", nil},
		{"-s", "required", []string{"open", "closed", "merged", "all"}},
		{"--state", "required", []string{"open", "closed", "merged", "all"}},
		{"--one", "required", []string{"a", "b"}}, {"--two", "required", []string{"c", "d"}},
		{"--format", "required", []string{"json", "text"}},
		{"--mode", "required", []string{"--fast", "--slow"}},
		{"--output", "required", nil}, {"--example", "required", nil}, {"--conflict", "required", nil},
		{"--broken", "unknown", nil},
	} {
		flag := command.Flag(tc.name)
		if flag == nil || flag.Value != tc.value || !slices.Equal(flag.Choices, tc.choices) {
			t.Errorf("%s: %+v; want %s %v", tc.name, flag, tc.value, tc.choices)
		}
	}
	if command.Flag("--fast") != nil || command.Flag("--slow") != nil {
		t.Fatal("choice values became flags")
	}
}

func TestOnlyDocumentedAliasesAreMapped(t *testing.T) {
	for _, tc := range []struct {
		help string
		want []string
	}{
		{"ALIASES\n  acme pr ls\n\nFLAGS\n  --help\n", []string{"ls"}},
		{"Aliases:\n  list, ls, l\n\n", []string{"ls", "l"}},
		{"ALIASES\n  acme other ls\n  aliases are useful\n", nil},
		{"Examples:\n  acme pr ls\n", nil},
	} {
		if got := helpAliases(tc.help, "acme", []string{"pr", "list"}); !slices.Equal(got, tc.want) {
			t.Errorf("aliases=%v, want=%v for %q", got, tc.want, tc.help)
		}
	}
	root := Command{Commands: []Command{
		{Name: "list", Aliases: []string{"ls", "conflict"}},
		{Name: "ls"},
		{Name: "other", Aliases: []string{"conflict"}},
	}}
	if root.Child("ls").Name != "ls" || root.Child("conflict") != nil {
		t.Fatal("aliases overrode a canonical name or an ambiguous alias was resolved")
	}
}

func TestAliasBudgetDoesNotPoisonNormalCollection(t *testing.T) {
	binary := helpExecutable(t, `
case "$*" in
  -h) printf 'Usage: acme COMMAND\nCommands:\n  alpha  Unavailable\n  good  Available via fallback\n' ;;
  'good --help') printf 'Usage: acme good\n' ;;
  *) echo 'error: unsupported help'; exit 2 ;;
esac
`)
	collector, err := NewHelpCollector(t.Context(), HelpOptions{Binary: binary, Version: "1.0.0", MaxCommands: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.CollectAliases(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if collector.aliasCalls != 3 || collector.calls != 1 {
		t.Fatalf("wrong budgets: normal=%d alias=%d", collector.calls, collector.aliasCalls)
	}
	if err := collector.Collect(t.Context(), []string{"good"}); err != nil {
		t.Fatalf("alias discovery poisoned a valid later request: %v", err)
	}
}
