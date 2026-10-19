package extract

import "testing"

func TestShellAliasAndUnlabelledTranscripts(t *testing.T) {
	for _, language := range []string{"shell", ""} {
		source := "```" + language + "\n# ripgrep-style transcript\n$ rg fast README.md\n75: faster than both\n$ rg -n slow README.md\n100: a result\n```\n"
		examples := Markdown("GUIDE.md", []byte(source))
		if len(examples) != 2 {
			t.Fatalf("%q: expected two prompted commands: %+v", language, examples)
		}
		for i, example := range examples {
			if example.CLI() != "rg" || example.Code != "" || example.Location != (Location{"GUIDE.md", 3 + i*2, 3}) {
				t.Errorf("%q: output became a command or location changed: %+v", language, example)
			}
		}
	}
	for _, source := range []string{
		"```\nrg fast README.md\n```\n",
		"```text\n$ rg fast README.md\n```\n",
		"```python\n$ not shell\n```\n",
	} {
		if examples := Markdown("data.md", []byte(source)); len(examples) != 0 {
			t.Errorf("unidentified or non-shell data was guessed to be shell: %+v", examples)
		}
	}
	if examples := Markdown("README.md", []byte("```shell\ngh repo clone cli/cli\n```\n")); len(examples) != 1 || examples[0].CLI() != "gh" {
		t.Fatalf("shell alias was not recognized: %+v", examples)
	}
}

func TestAmbiguousGreaterThanPromptsAreNotExecuted(t *testing.T) {
	for _, body := range []string{
		"> fd -e rs\nsrc/main.rs\n> fd -H pre-commit\n.git/hooks/pre-commit.sample\n",
		"> fd acme --output file\n", // Could also be a leading redirection, not a prompt.
	} {
		examples := Markdown("README.md", []byte("```bash\n"+body+"```\n"))
		if len(examples) != 1 || examples[0].Code != "unsupported-shell" || len(examples[0].Words) != 0 {
			t.Fatalf("ambiguous input/output was treated as executable commands: %+v", examples)
		}
	}
}

func TestDiagnosticOnlyConsoleBlock(t *testing.T) {
	for _, body := range []string{"WARNING: No swap limit support", "error: cannot connect", "Usage: program [OPTIONS]"} {
		examples := Markdown("README.md", []byte("```console\n"+body+"\n```\n"))
		if len(examples) != 1 || examples[0].Code != "unsupported-shell" || len(examples[0].Words) != 0 {
			t.Fatalf("terminal output became an executable: %+v", examples)
		}
	}
	if examples := Markdown("README.md", []byte("```console\ndocker run --rm ubuntu\n```\n")); len(examples) != 1 || examples[0].CLI() != "docker" {
		t.Fatalf("unprompted command was lost: %+v", examples)
	}
}
