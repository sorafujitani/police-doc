package extract

import (
	"strings"
	"testing"
)

func FuzzQuotedExecutableNames(f *testing.F) {
	f.Add("fake")
	f.Add("日本語 '")
	f.Fuzz(func(t *testing.T, prefix string) {
		if !singleLineWord(prefix) || strings.Contains(prefix, "/") {
			t.Skip()
		}
		for _, suffix := range []string{`\npx`, `\sudo`} {
			name := prefix + suffix
			body := "```bash\n" + shellQuote("./"+name) + " acme --verbose\n```\n"
			examples := Markdown("README.md", []byte(body))
			if len(examples) != 1 || examples[0].Reason != "" || examples[0].CLI() != name {
				t.Fatalf("quoted executable %q was reinterpreted: %+v", name, examples)
			}
		}
	})
}

func TestEnvironmentChangesDoNotSelectAnotherExecutable(t *testing.T) {
	for _, command := range []string{
		"PATH=/missing acme --verbose",
		"PATH+=:/missing acme --verbose",
		`PATH="$ROOT/bin" acme --verbose`,
		"PATH=/missing sudo acme --verbose",
		"PATH=/missing echo ok",
		"sudo FOO=bar acme --verbose",
		"sudo -- FOO=bar acme --verbose",
		"sudo PATH=/missing acme --verbose",
	} {
		examples := Markdown("README.md", []byte("```bash\n"+command+"\n```\n"))
		if len(examples) != 1 || examples[0].Code != "unsupported-environment" || examples[0].Reason == "" || examples[0].CLI() != "" {
			t.Errorf("environment change was silently ignored: %s: %+v", command, examples)
		}
	}
}

func TestQuotedBackslashesDoNotCreateWrappers(t *testing.T) {
	for _, name := range []string{`fake\npx`, `fake\sudo`} {
		command := "./'" + name + "' acme --verbose"
		example := Markdown("README.md", []byte("```bash\n"+command+"\n```\n"))[0]
		if example.Reason != "" || example.CLI() != name || example.LocalPackage || example.Words[0].Value != "./"+name {
			t.Errorf("literal executable was unwrapped: %+v", example)
		}
	}
	for _, command := range []string{"FOO=bar acme --verbose", "sudo -u root acme --verbose", "npx acme --verbose"} {
		example := Markdown("README.md", []byte("```bash\n"+command+"\n```\n"))[0]
		if example.Reason != "" || example.CLI() != "acme" {
			t.Errorf("ordinary command resolution regressed: %s: %+v", command, example)
		}
	}
}
