package extract

import "testing"

func TestShellBuiltinsAreNotExternalCommands(t *testing.T) {
	for _, command := range []string{"cd rfmt", "echo hello", "printf '%s' text", "pwd", "source ./env", "read -r answer", ":"} {
		t.Run(command, func(t *testing.T) {
			examples := Markdown("README.md", []byte("```sh\n"+command+" && acme status\n```\n"))
			if len(examples) != 2 || examples[0].Code != "shell-builtin" || examples[0].Reason == "" || examples[0].CLI() != "" {
				t.Fatalf("builtin remained an external command: %+v", examples)
			}
			if examples[1].CLI() != "acme" || examples[1].Reason != "" {
				t.Fatalf("lost the following external command: %+v", examples)
			}
		})
	}
}

func TestExplicitExternalCommandsAreNotBuiltins(t *testing.T) {
	for _, command := range []string{"/bin/echo hello", "./cd rfmt", "sudo echo hello", "npx cd rfmt"} {
		t.Run(command, func(t *testing.T) {
			examples := Markdown("README.md", []byte("```sh\n"+command+"\n```\n"))
			if len(examples) != 1 || examples[0].Reason != "" || examples[0].CLI() == "" {
				t.Fatalf("explicit external command was skipped: %+v", examples)
			}
		})
	}
}
