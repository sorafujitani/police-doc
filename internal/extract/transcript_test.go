package extract

import (
	"strings"
	"testing"
)

func TestTranscriptsIgnoreOutputInEveryShellFence(t *testing.T) {
	for _, language := range []string{"sh", "bash", "console"} {
		for _, newline := range []string{"\n", "\r\n"} {
			t.Run(language+newline, func(t *testing.T) {
				lines := []string{
					"```" + language,
					"# Commands and their output",
					"$ acme app/",
					"Processing 25 file(s)...",
					"✓ Formatted app/models/user.rb",
					"  (3 formatted, 22 unchanged)",
					"> not an input continuation",
					"",
					"  $ acme --quiet app/",
					"✓ 3 files formatted",
					"$ acme --verbose app/",
					"Details:",
					"  Total time: 0.45s",
					"acme --looks-like-a-command",
					"```",
				}
				examples := Markdown("README.md", []byte(strings.Join(lines, newline)))
				want := []Location{{"README.md", 3, 3}, {"README.md", 9, 5}, {"README.md", 11, 3}}
				if len(examples) != len(want) {
					t.Fatalf("output became examples: %+v", examples)
				}
				for i, example := range examples {
					if example.CLI() != "acme" || example.Reason != "" || example.Location != want[i] {
						t.Errorf("command %d: %+v", i, example)
					}
				}
			})
		}
	}
}

func TestTranscriptContinuations(t *testing.T) {
	cases := []struct {
		body string
		want []string
	}{
		{"$ acme export \\\n  --output out.txt", []string{"acme export --output out.txt"}},
		{"$ acme export \\\n> --output out.txt", []string{"acme export --output out.txt"}},
		{"$ acme export \\\n  --output \\\n> out.txt", []string{"acme export --output out.txt"}},
		{"$ acme status |\n> other --ok", []string{"acme status", "other --ok"}},
		{"$ acme status &&\n  other --ok", []string{"acme status", "other --ok"}},
		{"$ acme \"" + strings.Repeat("x", 4096) + "\"", []string{"acme " + strings.Repeat("x", 4096)}},
	}
	for _, language := range []string{"sh", "bash", "console"} {
		for _, tc := range cases {
			t.Run(language+tc.body[:min(len(tc.body), 50)], func(t *testing.T) {
				source := "```" + language + "\n" + tc.body + "\n✓ Done (output, not a command)\n```\n"
				examples := Markdown("README.md", []byte(source))
				if len(examples) != len(tc.want) {
					t.Fatalf("unexpected continuation parsing: %+v", examples)
				}
				for i, example := range examples {
					var words []string
					for _, word := range example.Words {
						words = append(words, word.Value)
					}
					if example.Reason != "" || strings.Join(words, " ") != tc.want[i] {
						t.Errorf("command %d: %+v, want %q", i, example, tc.want[i])
					}
				}
			})
		}
	}
}

// The shell parser can confuse a backslash at the end of a comment with a
// continuation. Do not report the following terminal output as invalid input.
func TestAmbiguousUnpromptedOutputIsUncheckable(t *testing.T) {
	source := "```bash\n$ acme status # comment \\\n✓ Done (output)\n```\n"
	examples := Markdown("README.md", []byte(source))
	if len(examples) != 1 || examples[0].Code != "unsupported-shell" || examples[0].CLI() != "" {
		t.Fatalf("ambiguous output became a command or a syntax warning: %+v", examples)
	}
}

func TestTranscriptMalformedCommandKeepsItsLocation(t *testing.T) {
	for _, language := range []string{"sh", "bash", "console"} {
		source := "```" + language + "\n$ acme status\noutput\n\n$ acme )\nmore output\neven more output\n```\n"
		examples := Markdown("README.md", []byte(source))
		want := Location{"README.md", 5, 8}
		if len(examples) != 1 || examples[0].Code != "invalid-shell" || examples[0].Location != want {
			t.Errorf("%s: invalid command was hidden or relocated: %+v", language, examples)
		}
	}
}
