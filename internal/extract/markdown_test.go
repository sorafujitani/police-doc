package extract

import (
	"strings"
	"testing"
)

func TestMarkdownPositionsAndShell(t *testing.T) {
	source := "intro `acme ignored`\n\n```bash\nacme publish \\\n  --output \"a b\" # comment\nacme status | other --x && acme \"$ACTION\"\nFOO=bar acme status\nONLY=assignment\n```\n\n```console\n$ acme status\nterminal output is not a command\n$ acme publish \\\n> doc.md\n```\n\n```go\nacme ignored\n```\n"
	examples := Markdown("README.md", []byte(source))
	want := []struct {
		line, col int
		words     []string
		dynamic   bool
	}{
		{4, 1, []string{"acme", "publish", "--output", "a b"}, false},
		{6, 1, []string{"acme", "status"}, false},
		{6, 15, []string{"other", "--x"}, false},
		{6, 28, []string{"acme", ""}, true},
		{7, 9, []string{"acme", "status"}, false},
		{12, 3, []string{"acme", "status"}, false},
		{14, 3, []string{"acme", "publish", "doc.md"}, false},
	}
	if len(examples) != len(want) {
		t.Fatalf("got %d examples: %#v", len(examples), examples)
	}
	for i, expected := range want {
		actual := examples[i]
		if actual.Location != (Location{File: "README.md", Line: expected.line, Column: expected.col}) {
			t.Errorf("example %d location: %+v", i, actual.Location)
		}
		if len(actual.Words) != len(expected.words) {
			t.Fatalf("example %d words: %#v", i, actual.Words)
		}
		for j, word := range actual.Words {
			if word.Value != expected.words[j] || word.Static != !(expected.dynamic && j == 1) {
				t.Errorf("example %d word %d: %+v", i, j, word)
			}
		}
	}
}

func TestNestedMarkdownAndCRLF(t *testing.T) {
	cases := []struct {
		name, source string
		line, column int
	}{
		{"indented", "  ```sh\n  acme status\n  ```\n", 2, 3},
		{"quote", "> ```sh\n> acme status\n> ```\n", 2, 3},
		{"list", "- example\n\n  ```sh\n  $ acme status\n  ```\n", 4, 5},
		{"quote-prompt", ">   ```console\n>   $ acme status\n>   output\n>   ```\n", 2, 7},
		{"crlf", "intro\r\n```sh\r\nacme status\r\n```\r\n", 3, 1},
		{"unicode-column", "```sh\nacme café && acme status\n```\n", 2, 14},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			examples := Markdown("doc.md", []byte(tc.source))
			if len(examples) == 0 {
				t.Fatal("no example extracted")
			}
			actual := examples[len(examples)-1].Location
			if actual.Line != tc.line || actual.Column != tc.column {
				t.Errorf("got %+v, want %d:%d", actual, tc.line, tc.column)
			}
		})
	}
}

func TestLiteralAndDynamicWords(t *testing.T) {
	cases := []struct {
		word, value string
		static      bool
	}{
		{`'a b'`, "a b", true},
		{`a\ b`, "a b", true},
		{`"a\qb"`, `a\qb`, true},
		{`"a\"b"`, `a"b`, true},
		{`\$HOME`, "$HOME", true},
		{`"*.md"`, "*.md", true},
		{`\*.md`, "*.md", true},
		{`$HOME`, "", false},
		{`"$HOME"`, "", false},
		{`$(touch NEVER_RUN)`, "", false},
		{"`touch NEVER_RUN`", "", false},
		{`*.md`, "", false},
		{`~/doc.md`, "", false},
		{`{one,two}`, "", false},
		{`$'escaped\n'`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.word, func(t *testing.T) {
			examples := Markdown("doc.md", []byte("```sh\nacme "+tc.word+"\n```\n"))
			if len(examples) != 1 || len(examples[0].Words) != 2 {
				t.Fatalf("unexpected examples: %#v", examples)
			}
			word := examples[0].Words[1]
			if word.Value != tc.value || word.Static != tc.static {
				t.Errorf("got %+v, want %q static=%v", word, tc.value, tc.static)
			}
		})
	}
}

func TestUncheckableShell(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{"acme 'unterminated", "invalid-shell"},
		{"for f in *.md; do acme publish \"$f\"; done", "unsupported-shell"},
		{"(acme status)", "unsupported-shell"},
	} {
		examples := Markdown("doc.md", []byte("```bash\n"+tc.body+"\n```\n"))
		if len(examples) != 1 || examples[0].Code != tc.code || examples[0].Reason == "" {
			t.Errorf("%s: %#v", tc.body, examples)
		}
	}
	if examples := Markdown("empty.md", []byte(strings.Repeat("\n", 10))); len(examples) != 0 {
		t.Fatalf("empty input produced examples: %#v", examples)
	}
}
