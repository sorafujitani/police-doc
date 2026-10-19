package extract

import "testing"

func TestCompoundBodiesAreInspectedWithoutEvaluation(t *testing.T) {
	for _, body := range []string{
		`for f in *.md; do acme publish "$f"; done`,
		`(acme publish "$f")`,
		`if acme ready; then acme publish "$f"; fi`,
		`while acme ready; do acme publish "$f"; done`,
		`run() { acme publish "$f"; }`,
		`case "$mode" in prod) acme publish "$f";; esac`,
		`time -p acme publish "$f"`,
	} {
		examples := Markdown("doc.md", []byte("```bash\n"+body+"\n```\n"))
		if len(examples) == 0 {
			t.Fatalf("lost body: %s", body)
		}
		for _, example := range examples {
			if example.CLI() != "acme" || example.Reason != "" {
				t.Errorf("body discarded: %+v", example)
			}
		}
	}
}

func TestBindingsAndControlSubstitutionsRemainVisible(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{`acme() { other; }; acme --bad`, "shell-binding"},
		{`alias acme='other'; acme --bad`, "shell-binding"},
		{`for f in $(unverified --bad); do acme "$f"; done`, "unchecked-substitution"},
		{`export PATH=/unknown`, "unsupported-environment"},
	} {
		examples := Markdown("doc.md", []byte("```bash\n"+tc.body+"\n```\n"))
		found := false
		for _, e := range examples {
			found = found || e.Code == tc.code
			if e.CLI() == "unverified" {
				t.Fatal("substitution became a live executable request")
			}
		}
		if !found {
			t.Errorf("missing %s: %+v", tc.code, examples)
		}
	}
}

func TestDeclarationAndBracketBuiltinExtraction(t *testing.T) {
	examples := Markdown("doc.md", []byte("```bash\nexport EXAMPLE='two words'\n[ -n \"$BASH\" ]\n````\n"))
	if len(examples) != 2 || examples[0].Code != "shell-builtin" || examples[0].Words[1].Value != "EXAMPLE=two words" || examples[1].Code != "shell-builtin" {
		t.Fatalf("declaration or bracket syntax lost: %+v", examples)
	}
}

func TestDeclarationRedirectionSubstitutionIsNotLost(t *testing.T) {
	examples := Markdown("doc.md", []byte("```bash\nexport X=1 >\"$(unverified --bad)\"\n```\n"))
	if len(examples) != 1 || !examples[0].HasSubstitution {
		t.Fatalf("lost substitution: %+v", examples)
	}
}

func TestChevronTranscriptKeepsUnicodeLocations(t *testing.T) {
	examples := Markdown("doc.md", []byte("```sh\n❯ acme first\n├── a\n└── b\n❯ acme second\noutput\n```\n"))
	if len(examples) != 2 {
		t.Fatalf("output became a command: %+v", examples)
	}
	for i, e := range examples {
		if e.CLI() != "acme" || e.Location.Column != 3 || e.Location.Line != 2+i*3 {
			t.Errorf("wrong command or location: %+v", e)
		}
	}
}
