package extract

import (
	"strings"
	"testing"
)

func TestWrapperResolution(t *testing.T) {
	for _, tc := range []struct {
		command, want string
		local         bool
	}{
		{"sudo git status", "git status", false},
		{"sudo -u root -- git commit -m hello", "git commit -m hello", false},
		{"sudo -n --user=root /usr/bin/git status", "/usr/bin/git status", false},
		{"sudo -uroot git status", "git status", false},
		{"npx eslint .", "eslint .", true},
		{"npx --no-install eslint --output out", "eslint --output out", true},
		{"npx -y -- eslint .", "eslint .", true},
		{"sudo -u root npx eslint .", "eslint .", true},
	} {
		t.Run(tc.command, func(t *testing.T) {
			examples := Markdown("README.md", []byte("```sh\n"+tc.command+"\n```\n"))
			if len(examples) != 1 {
				t.Fatal(examples)
			}
			example := examples[0]
			var words []string
			for _, word := range example.Words {
				words = append(words, word.Value)
			}
			if example.Reason != "" || strings.Join(words, " ") != tc.want || example.LocalPackage != tc.local || example.Text != tc.command || example.Location.Line != 2 {
				t.Fatalf("unexpected resolution: %+v", example)
			}
		})
	}
	for _, command := range []string{
		`sudo "$CLI" status`, `sudo -u "$USER" git status`, "sudo -u", "sudo", "npx",
		"sudo --unknown git status", "sudo -i git status", "sudo -D /tmp git status",
		"npx --package eslint eslint .", "npx -c 'eslint .'", "npx eslint@9 .", "npx @scope/tool .", "npx ./tool .",
	} {
		example := Markdown("README.md", []byte("```sh\n"+command+"\n```\n"))[0]
		if example.Code != "unsupported-wrapper" || example.Reason == "" || example.CLI() != "" {
			t.Errorf("unsafe wrapper resolution: %s: %+v", command, example)
		}
	}
}
