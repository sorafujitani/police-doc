package spec

import (
	"slices"
	"testing"
)

func TestExamplesDoNotAdvertiseFlags(t *testing.T) {
	command := parseHelp(`Usage: acme [OPTIONS]

OPTIONS
  --real  A real flag
  For example:
      --nested-example VALUE
  --after  Another real flag

EXAMPLES
  --example VALUE

SEE ALSO
  --prose VALUE
`)
	for _, name := range []string{"--real", "--after"} {
		if command.Flag(name) == nil {
			t.Errorf("lost real flag %s", name)
		}
	}
	for _, name := range []string{"--nested-example", "--example", "--prose"} {
		if command.Flag(name) != nil {
			t.Errorf("example/prose was treated as a flag declaration: %s", name)
		}
	}
}

func TestHeaderlessFlagsAndDescriptionsEndingInColon(t *testing.T) {
	command := parseHelp(`Usage: acme [OPTIONS]

This is a shortcut for:

acme another-command

      --long FILE  A long-only option
  -f, --force  A description ending with an environment marker:
                 VARIABLE=]
`)
	for _, name := range []string{"--long", "-f", "--force"} {
		if command.Flag(name) == nil {
			t.Errorf("lost headerless flag or colon-ended definition: %s", name)
		}
	}
}

func TestIndentedIntroductionBeforeFlags(t *testing.T) {
	command := parseHelp("Usage: acme [OPTIONS]\n\n  The following options are supported:\n    --output FILE  Output file\n")
	if flag := command.Flag("--output"); flag == nil || flag.Value != "required" {
		t.Fatalf("introductory paragraph hid a declared option: %+v", command)
	}
	command = parseHelp("Options:\n  For example:\n    --not-a-flag FILE\n  --real FILE  Real option\n")
	if command.Flag("--not-a-flag") != nil || command.Flag("--real") == nil {
		t.Fatalf("an example before the first declaration was not isolated: %+v", command)
	}
}

func TestChoiceSetsIgnoreOrderAndExamples(t *testing.T) {
	command := parseHelp(`Options:
  --state {open,closed}  States
  --state {closed,open}  Same states, different order
  --template string  For example: {title|author}
  --default string  Default: {red|green}
  --nested-default string  Template (default: {title|author})
  --explicit string  Allowed values (default red): {red|green}
`)
	state := command.Flag("--state")
	if state == nil || len(state.Choices) != 2 || !slices.Contains(state.Choices, "open") || !slices.Contains(state.Choices, "closed") {
		t.Fatalf("choice ordering erased the constraint: %+v", state)
	}
	for _, name := range []string{"--template", "--default", "--nested-default"} {
		if flag := command.Flag(name); flag == nil || len(flag.Choices) != 0 {
			t.Errorf("illustration became a choice restriction: %+v", flag)
		}
	}
	if flag := command.Flag("--explicit"); flag == nil || len(flag.Choices) != 2 {
		t.Fatalf("explicit choices with a default note were lost: %+v", flag)
	}
}

func TestAliasesTolerateSpacingButNotCommaSeparatedProse(t *testing.T) {
	for _, tc := range []struct {
		help string
		want []string
	}{
		{"ALIASES\n\n  acme pr ls\n\nFLAGS\n", []string{"ls"}},
		{"Aliases:\n  ls, l\n\n", []string{"ls", "l"}},
		{"Aliases:\n  ls, then choose a format\n\n", nil},
		{"Aliases:\n  use this, ls\n\n", nil},
		{"ALIASES\n  LS\n\nFLAGS\n", []string{"LS"}},
	} {
		if got := helpAliases(tc.help, "acme", []string{"pr", "list"}); !slices.Equal(got, tc.want) {
			t.Errorf("%q produced aliases %v, want %v", tc.help, got, tc.want)
		}
	}
}

func TestIndentedProseDoesNotOverrideFlagDefinitions(t *testing.T) {
	command := parseHelp(`Usage: acme [OPTIONS]

OPTIONS
     --color[=WHEN]
         --color=always. This is explanatory prose.
         --not-a-flag VALUE
     --stat[=WIDTH]
`)
	if flag := command.Flag("--color"); flag == nil || flag.Value != "optional-attached" {
		t.Fatalf("indented prose changed the flag's value rule: %+v", flag)
	}
	if command.Flag("--not-a-flag") != nil {
		t.Fatal("indented prose became a declared flag")
	}
	command = parseHelp("Options:\n      --long FILE\n  -s, --short FILE\n      --last FILE\n")
	if len(command.Flags) != 4 {
		t.Fatalf("aligned getopt columns lost declarations: %+v", command.Flags)
	}
}

func TestExplicitAttachedOptionalValue(t *testing.T) {
	command := parseHelp("Options:\n  --color[=WHEN]  Color mode\n  --maybe[<VALUE>]  Ambiguous optional value\n")
	if flag := command.Flag("--color"); flag == nil || flag.Value != "optional-attached" {
		t.Fatalf("attached-only optional value lost: %+v", flag)
	}
	if flag := command.Flag("--maybe"); flag == nil || flag.Value != "optional" {
		t.Fatalf("ambiguous optional value became attached-only: %+v", flag)
	}
}
