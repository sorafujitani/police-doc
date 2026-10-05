package extract

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func wordExample(t *testing.T, word string) Word {
	t.Helper()
	examples := Markdown("fuzz.md", []byte("```bash\nacme "+word+"\n```\n"))
	if len(examples) != 1 || examples[0].Reason != "" || len(examples[0].Words) != 2 {
		t.Fatalf("word %q did not produce one two-word command: %+v", word, examples)
	}
	return examples[0].Words[1]
}

func singleLineWord(value string) bool {
	return len(value) <= 4096 && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

// Shell quoting must preserve arbitrary literal data, not reinterpret it as syntax.
func FuzzQuotedWords(f *testing.F) {
	for _, value := range []string{"", "a b", "'\\\"$`", "{a,b}", "*?[", "日本語", "\t", "~"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if !singleLineWord(value) {
			t.Skip()
		}
		double := "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "`", "\\`").Replace(value) + "\""
		want := Word{Value: value, Static: true}
		for _, quoted := range []string{shellQuote(value), double} {
			if got := wordExample(t, quoted); got != want {
				t.Fatalf("%q: got %+v, want %+v", quoted, got, want)
			}
		}
	})
}

// Quoting the alternatives does not disable unquoted brace delimiters.
func FuzzBraceWords(f *testing.F) {
	f.Add("a", "b")
	f.Add("", "")
	f.Add("a,b", "{c,d}")
	f.Fuzz(func(t *testing.T, left, right string) {
		if !singleLineWord(left) || !singleLineWord(right) {
			t.Skip()
		}
		word := "{" + shellQuote(left) + "," + shellQuote(right) + "}"
		if got := wordExample(t, word); got.Static {
			t.Fatalf("brace expansion %q was accepted as literal %+v", word, got)
		}
	})
}

// Commands inside redirection substitutions are not independent CLI examples.
func FuzzRedirectionSubstitutions(f *testing.F) {
	f.Add("status")
	f.Add("--removed")
	f.Fuzz(func(t *testing.T, value string) {
		if !singleLineWord(value) {
			t.Skip()
		}
		for _, redirect := range []string{
			"> \"$(other " + shellQuote(value) + ")\"",
			"< <(other " + shellQuote(value) + ")",
			"<<EOF\n$(other " + shellQuote(value) + ")\nEOF",
		} {
			body := "acme status " + redirect
			examples := Markdown("fuzz.md", []byte("```bash\n"+body+"\n```\n"))
			if len(examples) != 1 || examples[0].Reason != "" || examples[0].Words[0] != (Word{Value: "acme", Static: true}) {
				t.Fatalf("nested substitution became a separate example: %+v", examples)
			}
		}
	})
}

// Arbitrary Markdown and shell bytes must not panic or produce out-of-range locations.
func FuzzMarkdownLocations(f *testing.F) {
	for _, source := range []string{"", "```sh\nacme status\n```", "> ```console\n> $ acme café && acme --x\n> ```\n", "acme '\x00", "\xff\r\n", "acme <<EOF\n$ value\nEOF"} {
		f.Add([]byte(source))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 16384 {
			t.Skip()
		}
		for _, source := range []string{string(data), "```bash\n" + string(data) + "\n```\n"} {
			examples := Markdown("fuzz.md", []byte(source))
			if !reflect.DeepEqual(examples, Markdown("fuzz.md", []byte(source))) {
				t.Fatal("extraction is nondeterministic")
			}
			lines := strings.Split(source, "\n")
			for _, example := range examples {
				loc := example.Location
				if loc.File != "fuzz.md" || loc.Line < 1 || loc.Line > len(lines) {
					t.Fatalf("out-of-range location %+v in %q", loc, source)
				}
				if loc.Column < 1 || loc.Column > utf8.RuneCountInString(lines[loc.Line-1])+1 {
					t.Fatalf("out-of-range column %+v in %q", loc, source)
				}
			}
		}
	})
}
