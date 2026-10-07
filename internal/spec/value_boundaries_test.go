package spec

import (
	"slices"
	"testing"
)

func TestFixedValueSlots(t *testing.T) {
	for _, tc := range []struct {
		signature, mode string
		count           int
	}{
		{" LEFT RIGHT", "required", 2},
		{" <LEFT> <RIGHT>", "required", 2},
		{" LEFT MIDDLE RIGHT", "required", 3},
		{" FILE] [PATH]", "required", 0},
		{" LEFT RIGHT] [PATH]", "required", 2},
		{" {red, blue}", "required", 0},
		{"=filename", "required", 0},
		{"=...", "required", 0},
		{"[=WHEN]", "optional-attached", 0},
		{" FILE [EXTRA]", "unknown", 0},
		{" FILE...", "unknown", 0},
		{" FILE ...", "unknown", 0},
	} {
		mode, count := helpValueArity(tc.signature)
		if mode != tc.mode || count != tc.count {
			t.Errorf("%q: %s/%d, want %s/%d", tc.signature, mode, count, tc.mode, tc.count)
		}
	}
	command := parseHelp("Options:\n  -p, --pair LEFT RIGHT\n  --pair  Pair: {a,b}\n")
	for _, name := range []string{"-p", "--pair"} {
		flag := command.Flag(name)
		if flag == nil || flag.ValueCount != 2 || len(flag.Choices) != 0 {
			t.Errorf("lost arity or inferred a shared choice set: %+v", flag)
		}
	}
	command = parseHelp("Options:\n  --pair {a,b}\n  --pair LEFT RIGHT\n")
	if flag := command.Flag("--pair"); flag.Value != "unknown" || flag.ValueCount != 0 || len(flag.Choices) != 0 {
		t.Fatalf("conflicting arities remained authoritative: %+v", flag)
	}
}

func TestChoiceConstraintsNeedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, signature, description string
		want                         []string
	}{
		{"--output", " FILE] {a,b}", "Output", nil},
		{"--format", "[={json,yaml}] FILE", "Format", []string{"json", "yaml"}},
		{"--point", " <X,Y>", "Coordinates", nil},
		{"--source", " <FILE|URL>", "Input", nil},
		{"--point", " <x,y>", "Coordinates", nil},
		{"--template", " TEXT", "Pattern, e.g.: {title|author}", nil},
		{"--template", " TEXT", "For instance: {title|author}", nil},
		{"--template", " TEXT", "Some choices, e.g.: {title|author}", nil},
		{"--state", " string", "State: {open|closed}", []string{"open", "closed"}},
		{"--state", " string", "Filter by state: {open|closed}", []string{"open", "closed"}},
		{"--omit", " <dev|optional|peer>", "Omit dependencies", []string{"dev", "optional", "peer"}},
		{"--signal", " SIGNAL", "Allowed values: <TERM,KILL>", []string{"TERM", "KILL"}},
		{"--signal", " {TERM,KILL}", "Signal", []string{"TERM", "KILL"}},
	} {
		if got := helpFlagChoices(tc.name, tc.signature, tc.description); !slices.Equal(got, tc.want) {
			t.Errorf("%s %q / %q: %v, want %v", tc.name, tc.signature, tc.description, got, tc.want)
		}
	}
}

func TestValueCountsAreValidated(t *testing.T) {
	for _, tc := range []struct {
		mode  string
		count int
		valid bool
	}{
		{"required", 0, true}, {"required", 2, true},
		{"required", -1, false}, {"none", 2, false}, {"unknown", 2, false},
	} {
		snapshot := Snapshot{Tool: "acme", Version: "1.0.0", Root: Command{Flags: []Flag{{Name: "--pair", Value: tc.mode, ValueCount: tc.count}}}}
		if err := snapshot.Validate(); (err == nil) != tc.valid {
			t.Errorf("%s/%d: %v", tc.mode, tc.count, err)
		}
	}
}
