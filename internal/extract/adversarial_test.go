package extract

import (
	"fmt"
	"strings"
	"testing"
)

func TestMarkdownPreservesLiteralPromptText(t *testing.T) {
	for _, body := range []string{
		"cat <<'EOF'\n$ EOF\nacme publish --output\nEOF",
		"cat <<EOF\n$ EOF\nacme publish --output\nEOF",
		"printf '%s' '\n$ value\n> value\n'",
		"printf \"%s\" \"\n$ value\n> value\n\"",
	} {
		t.Run(body, func(t *testing.T) {
			examples := Markdown("doc.md", []byte("```bash\n"+body+"\n```\n"))
			if len(examples) != 1 || examples[0].Reason != "" {
				t.Fatalf("literal text became commands: %+v", examples)
			}
			if strings.HasPrefix(body, "printf") && examples[0].Words[2].Value != "\n$ value\n> value\n" {
				t.Fatalf("literal was changed: %+v", examples[0].Words)
			}
		})
	}
}

func TestMarkdownRejectsAmbiguousPromptedLiterals(t *testing.T) {
	for _, language := range []string{"bash", "console"} {
		for _, body := range []string{
			"$ cat <<'EOF'\n$ EOF\nacme publish --output\nEOF",
			"$ printf '%s' '\n$ value\n> value\n'",
		} {
			examples := Markdown("doc.md", []byte("```"+language+"\n"+body+"\n```\n"))
			if len(examples) != 1 || examples[0].Code != "unsupported-shell" || examples[0].Reason == "" || examples[0].CLI() != "" {
				t.Fatalf("ambiguous transcript became executable examples: %+v", examples)
			}
		}
	}
}

func TestMarkdownMalformedPromptStillWarns(t *testing.T) {
	for _, language := range []string{"bash", "console"} {
		examples := Markdown("doc.md", []byte("```"+language+"\n$ acme 'unterminated\n```"))
		if len(examples) != 1 || examples[0].Code != "invalid-shell" {
			t.Fatalf("invalid shell was hidden: %+v", examples)
		}
	}
}

func TestMarkdownLongLinePositions(t *testing.T) {
	const count = 4096
	examples := Markdown("doc.md", []byte("```sh\n"+strings.Repeat("acme café;", count)+"\n```"))
	if len(examples) != count {
		t.Fatalf("got %d examples, want %d", len(examples), count)
	}
	for i, example := range examples {
		if example.Location.Line != 2 || example.Location.Column != i*10+1 {
			t.Fatalf("example %d has location %+v", i, example.Location)
		}
	}
}

func BenchmarkMarkdownLongLine(b *testing.B) {
	for _, count := range []int{1000, 10000, 20000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			source := []byte("```sh\n" + strings.Repeat("acme café;", count) + "\n```")
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if len(Markdown("doc.md", source)) != count {
					b.Fatal("lost examples")
				}
			}
		})
	}
}
