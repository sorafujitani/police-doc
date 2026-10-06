package extract

import (
	"path"
	"strings"
)

// CLI returns the static executable name after wrapper resolution, not its path.
func (example Example) CLI() string {
	if example.Reason != "" || len(example.Words) == 0 || !example.Words[0].Static {
		return ""
	}
	return path.Base(strings.ReplaceAll(example.Words[0].Value, "\\", "/"))
}

// unwrap only interprets known wrapper syntax; it never invokes a wrapper.
func unwrap(example Example) Example {
	if len(example.Words) > 0 && example.Words[0].Static && shellBuiltin(example.Words[0].Value) {
		example.Code, example.Reason = "shell-builtin", "Shell builtins are not checked against external CLI help."
		return example
	}
	for example.CLI() == "sudo" || example.CLI() == "npx" {
		wrapper := example.CLI()
		index := 1
		for index < len(example.Words) {
			word := example.Words[index]
			if !word.Static {
				break
			}
			if word.Value == "--" {
				index++
				break
			}
			if !strings.HasPrefix(word.Value, "-") {
				break
			}
			name, _, attached := strings.Cut(word.Value, "=")
			value := false
			known := false
			if wrapper == "sudo" {
				switch name {
				case "-n", "--non-interactive", "-E", "--preserve-env", "-H", "--set-home", "-S", "--stdin", "-A", "--askpass", "-b", "--background", "-k", "--reset-timestamp":
					known = !attached || name == "--preserve-env"
				case "-u", "--user", "-g", "--group":
					known, value = true, true
				}
				if !known && len(name) > 2 && (strings.HasPrefix(name, "-u") || strings.HasPrefix(name, "-g")) && !attached {
					known, attached = true, true
				}
			} else {
				switch name {
				case "-y", "--yes", "--no", "--no-install":
					known = !attached
				}
			}
			if !known {
				example.Code, example.Reason = "unsupported-wrapper", "Cannot safely resolve "+wrapper+" option "+word.Value+"; the wrapper was not executed."
				return example
			}
			index++
			if value && !attached {
				if index >= len(example.Words) || !example.Words[index].Static {
					break
				}
				index++
			}
		}
		if index >= len(example.Words) || !example.Words[index].Static || strings.HasPrefix(example.Words[index].Value, "-") {
			example.Code, example.Reason = "unsupported-wrapper", "The wrapped executable is missing or dynamic; the wrapper was not executed."
			return example
		}
		if wrapper == "npx" {
			command := example.Words[index].Value
			if strings.ContainsAny(command, "@/\\=") {
				example.Code, example.Reason = "unsupported-wrapper", "npx package versions, package-to-binary mappings and paths cannot be resolved safely; no package was installed."
				return example
			}
			example.LocalPackage = true
		}
		example.Words = example.Words[index:]
	}
	return example
}

// Match bare names before unwrapping: /bin/echo and sudo echo select external
// executables, even though an unqualified echo normally selects a shell builtin.
func shellBuiltin(name string) bool {
	switch name {
	case ".", ":", "[", "alias", "bg", "bind", "break", "builtin", "caller",
		"cd", "command", "compgen", "complete", "compopt", "continue", "declare",
		"dirs", "disown", "echo", "enable", "eval", "exec", "exit", "export",
		"false", "fc", "fg", "getopts", "hash", "help", "history", "jobs", "kill",
		"let", "local", "logout", "mapfile", "popd", "printf", "pushd", "pwd",
		"read", "readarray", "readonly", "return", "set", "shift", "shopt",
		"source", "suspend", "test", "times", "trap", "true", "type", "typeset",
		"ulimit", "umask", "unalias", "unset", "wait":
		return true
	}
	return false
}
