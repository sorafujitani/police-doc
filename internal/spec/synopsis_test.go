package spec

import "testing"

func FuzzSynopsisFormsStayValid(f *testing.F) {
	for _, usage := range []string{"[OPTIONS] FILE [PATH ...]", "[-q FILE]", "--files --files", "[[FILE]]", "[SRC DST]...", "--output <file>"} {
		f.Add(usage)
	}
	f.Fuzz(func(t *testing.T, usage string) {
		if len(usage) > 32768 {
			t.Skip()
		}
		command := Command{Flags: []Flag{{Name: "--files", Value: "none"}, {Name: "-q", Value: "none"}, {Name: "--output", Value: "required"}}}
		command.Forms = synopsisForms("Usage: acme "+usage, "acme", nil, &command)
		if err := validateCommand(&command, "acme"); err != nil {
			t.Fatalf("inferred an invalid grammar for %q: %v", usage, err)
		}
	})
}

func TestSynopsisBounds(t *testing.T) {
	for _, tc := range []struct {
		usage          string
		accept, reject []int
	}{
		{"Usage: acme [OPTIONS] <source> <dest>", []int{2}, []int{0, 1, 3}},
		{"Usage: acme [FILE]...", []int{0, 1, 10}, nil},
		{"Usage: acme FILE [FILE ...]", []int{1, 2, 10}, []int{0}},
		{"Usage: acme [SRC DST]", []int{0, 2}, []int{1, 3}},
		{"Usage: acme [-o FILE] [PATH]", []int{0, 1}, []int{2}},
		{"Usage: acme LAST\n  or: acme FIRST LAST\n  or: acme FIRST INCREMENT LAST", []int{1, 2, 3}, []int{0, 4}},
	} {
		t.Run(tc.usage, func(t *testing.T) {
			command := Command{Flags: []Flag{{Name: "-o", Value: "required"}}}
			command.Forms = synopsisForms(tc.usage, "acme", nil, &command)
			if len(command.Forms) == 0 {
				t.Fatal("explicit syntax was not recognized")
			}
			for _, count := range tc.accept {
				if !command.FormAccepts(count, nil) {
					t.Errorf("rejected %d: %+v", count, command.Forms)
				}
			}
			for _, count := range tc.reject {
				if command.FormAccepts(count, nil) {
					t.Errorf("accepted %d: %+v", count, command.Forms)
				}
			}
		})
	}
}

func TestSynopsisModesAndAliases(t *testing.T) {
	text := "Usage: acme PATTERN [PATH ...]\n       acme -e PATTERN ... [PATH ...]\n       acme --files [PATH ...]\nOptions:\n  -e, --regexp PATTERN  Pattern\n  --files  List files\n"
	command, err := parseHelpPage(text, "acme", "acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		count int
		flag  string
		want  bool
	}{{0, "", false}, {1, "", true}, {0, "--regexp", true}, {0, "--files", true}, {2, "--regexp", true}} {
		if got := command.FormAccepts(tc.count, map[string]bool{tc.flag: true}); got != tc.want {
			t.Errorf("%+v: got %v forms=%+v flags=%+v", tc, got, command.Forms, command.Flags)
		}
	}
	if !command.FlagOrderUnknown {
		t.Fatal("synopsis placement invented an ordering rule")
	}
}

func TestOptionalFlagCannotLoseItsOperandDependency(t *testing.T) {
	command := Command{Flags: []Flag{{Name: "-q", Value: "none"}}}
	for _, text := range []string{"Usage: acme [OPTIONS] [-q FILE]", "Usage: acme [-q [FILE]]"} {
		if forms := synopsisForms(text, "acme", nil, &command); len(forms) != 0 {
			t.Fatalf("optional group lost flag/operand dependency: %+v", forms)
		}
	}
}

func TestAmbiguousSynopsisHasNoInventedBounds(t *testing.T) {
	for _, usage := range []string{"[COMMAND]", "[arguments]", "[FILE|-]", "[SRC DST]...", "[FILE", "FILE ]", "[build flags]", "-- COMMAND [ARGS]", "FILE\n  or: acme (anything)", ""} {
		text := "Usage: acme " + usage
		if usage == "" {
			text = "Usage:"
		}
		command := Command{}
		if forms := synopsisForms(text, "acme", nil, &command); len(forms) > 0 {
			t.Errorf("guessed bounds for %q: %+v", text, forms)
		}
	}
}
