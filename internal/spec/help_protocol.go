package spec

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Protocol differences belong here, not in traversal, caching or diagnostics.
// Every request is metadata-only; names come from advertised help, never argv.
func helpRequests(binary string, path []string) [][]string {
	switch strings.TrimSuffix(filepath.Base(binary), ".exe") {
	case "go":
		return [][]string{append([]string{"help"}, path...)}
	case "nix":
		return [][]string{append(slices.Clone(path), "--help")}
	case "curl":
		return [][]string{{"--help", "all"}}
	case "brew":
		if len(path) == 0 {
			return [][]string{{"commands", "--quiet"}}
		}
		return [][]string{append([]string{"help"}, path...)}
	default:
		return [][]string{
			append(slices.Clone(path), "-h"),
			append(slices.Clone(path), "--help"),
		}
	}
}

func helpOutput(ctx context.Context, binary string, args []string) (string, error) {
	if isGo(binary) {
		return goOutput(ctx, binary, args...)
	}
	return commandOutput(ctx, binary, helpEnvironment, true, args...)
}

func parseHelpPage(output, binary, tool string, path []string) (Command, error) {
	switch strings.TrimSuffix(filepath.Base(binary), ".exe") {
	case "nix":
		output = normalizeNixHelp(output, path)
	case "xattr":
		output = normalizeXattrHelp(output)
	case "rg":
		output = strings.ReplaceAll(output, "\nOTHER BEHAVIORS:\n", "\nOTHER OPTIONS:\n")
	case "fzf":
		// Long fzf signatures can occupy the whole heading column, leaving a
		// single space before prose. Restore that boundary, not the value.
		lines := strings.Split(output, "\n")
		for i, line := range lines {
			trimmed := strings.TrimLeft(line, " \t")
			head, description, found := strings.Cut(trimmed, " ")
			if found && strings.HasPrefix(head, "--") && strings.Contains(head, "=") && strings.Count(head, "[") == strings.Count(head, "]") && strings.Count(head, "<") == strings.Count(head, ">") {
				lines[i] = line[:len(line)-len(trimmed)] + head + "  " + description
			}
		}
		output = strings.Join(lines, "\n")
	}
	parsed := parseHelp(output)
	switch strings.TrimSuffix(filepath.Base(binary), ".exe") {
	case "brew":
		if len(path) == 0 {
			for line := range strings.SplitSeq(output, "\n") {
				if name := strings.TrimSpace(line); helpCommandName.MatchString(name) {
					helpChild(&parsed, name)
				}
			}
		}
	case "go":
		if len(path) == 0 {
			reading := false
			for line := range strings.SplitSeq(output, "\n") {
				if strings.TrimSpace(line) == "The commands are:" {
					reading = true
					continue
				}
				if strings.TrimSpace(line) == "Additional help topics:" {
					break
				}
				words := strings.Fields(line)
				if reading && strings.HasPrefix(line, "\t") && len(words) > 1 && helpCommandName.MatchString(words[0]) {
					helpChild(&parsed, words[0])
				}
			}
		}
		// Go puts some lowercase value names in flag headings and -o in Usage.
		for line := range strings.SplitSeq(output, "\n") {
			if match := goFlagLine.FindStringSubmatch(line); match != nil && strings.TrimSpace(match[2]) != "" {
				if flag := parsed.Flag(match[1]); flag != nil {
					flag.Value = "required"
				}
			}
		}
		if strings.HasPrefix(output, "usage: go build [-o output]") {
			if flag := parsed.Flag("-o"); flag != nil {
				flag.Value = "required"
			}
		}
	}
	expectedName := tool
	if len(path) == 0 {
		// Root help establishes the display name; launchers may print the
		// canonical interpreter or an executable alias instead of argv[0].
		expectedName = ""
	}
	usage := helpUsageInfo(output, expectedName, path)
	parsed.UsageName, parsed.SubcommandFirst = usage.name, usage.subcommandFirst
	parsed.FlagsAfterPositionals = usage.flagsAfterPositionals
	parsed.FlagOrderUnknown = !usage.flagsAfterPositionals
	parsed.Forms = synopsisForms(output, usage.name, path, &parsed)
	parsed.DerivedForms = len(parsed.Forms) > 0
	parsed.Aliases = helpAliases(output, usage.name, path)
	// Some root pages (notably npm) show example invocations instead of a
	// root synopsis. Their explicit command catalog still identifies the root.
	if helpUsageHeading.MatchString(output) && !usage.matched && (len(path) > 0 || len(parsed.Commands) == 0) {
		return Command{}, fmt.Errorf("help Usage does not describe the requested command unambiguously: %q", strings.Join(append([]string{tool}, path...), " "))
	}
	// Go install's help explicitly delegates its flag descriptions to build.
	// This establishes command usage, not the flags in that other help page.
	referencedFlags := isGo(binary) && strings.Contains(output, "[build flags]") && strings.Contains(output, "For more about build flags, see 'go help build'.")
	if len(parsed.Flags) == 0 && len(parsed.Commands) == 0 && (!usage.matched || (usage.needsDetails && !referencedFlags)) {
		return Command{}, fmt.Errorf("could not recognize help usage, flags or commands")
	}
	return parsed, nil
}

// Nix renders man-page sections and bullet lists instead of a Usage/Options
// table. Translate only declarations; examples and prose are not flag evidence.
func normalizeNixHelp(output string, path []string) string {
	var page strings.Builder
	section := ""
	prefix := strings.Join(append([]string{"nix"}, path...), " ")
	for line := range strings.SplitSeq(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len(path) > 0 {
			alias, target, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(trimmed, "│")), " is an alias for ")
			words := strings.Fields(alias)
			if ok && strings.TrimSuffix(target, ".") == prefix && len(words) == len(path)+1 && words[0] == "nix" && slices.Equal(words[1:len(path)], path[:len(path)-1]) && helpCommandName.MatchString(words[len(path)]) {
				fmt.Fprintf(&page, "\nAliases:\n  %s\n", words[len(path)])
			}
		}
		switch {
		case trimmed == "Synopsis":
			section = "synopsis"
			continue
		case trimmed == "where subcommand is one of the following:":
			section = "commands"
			continue
		case trimmed == "Options" || strings.HasPrefix(trimmed, "Options for ") || strings.HasPrefix(trimmed, "Options that "):
			section = "options"
			continue
		case section == "options" && strings.HasPrefix(trimmed, "## ") && (helpOptionHeading.MatchString(strings.TrimPrefix(trimmed, "## ")) || strings.HasPrefix(trimmed, "## Options that ")):
			continue
		}
		if section == "synopsis" && strings.HasPrefix(trimmed, "nix ") {
			words := strings.Fields(trimmed)
			for i := len(path) + 1; i < len(words); i++ {
				switch {
				case words[i] == "[option...]":
					words[i] = "[options]"
				case strings.HasPrefix(words[i], "-"):
				default:
					words[i] = strings.ToUpper(words[i])
				}
			}
			fmt.Fprintf(&page, "Usage: %s\n\n", strings.Join(words, " "))
			section = ""
			continue
		}
		bullet, isBullet := strings.CutPrefix(trimmed, "· ")
		if isBullet && section == "commands" {
			invocation, description, ok := strings.Cut(bullet, " - ")
			name, matches := strings.CutPrefix(invocation, prefix+" ")
			if ok && matches && helpCommandName.MatchString(name) {
				fmt.Fprintf(&page, "\nCommands:\n  %s  %s\n", name, description)
			}
		} else if isBullet && section == "options" && strings.HasPrefix(bullet, "-") {
			words := strings.Fields(bullet)
			for i, word := range words {
				if !strings.HasPrefix(word, "-") && word != "/" {
					words[i] = strings.ToUpper(word)
				}
			}
			fmt.Fprintf(&page, "\nOptions:\n  %s\n", strings.Join(words, " "))
		} else if line == trimmed && !(section == "commands" && (strings.HasSuffix(trimmed, ":") || strings.Contains(strings.ToLower(trimmed), "commands"))) {
			section = ""
		}
	}
	return page.String()
}

func normalizeXattrHelp(output string) string {
	var page, flags strings.Builder
	operands := strings.NewReplacer("attr_name", "ATTR_NAME", "attr_value", "ATTR_VALUE", "file", "FILE")
	for _, words := range helpSynopses(output) {
		if len(words) == 0 || words[0] != "xattr" {
			continue
		}
		fmt.Fprintf(&page, "Usage: xattr %s\n\n", operands.Replace(strings.Join(words[1:], " ")))
		// xattr's -p/-w/-d/-c modes appear only in its usage alternatives.
		// Attribute names and files remain unchecked positional operands.
		for _, word := range words[1:] {
			if name := strings.Trim(word, "[]"); len(name) == 2 && name[0] == '-' {
				fmt.Fprintf(&flags, "  %s\n", name)
			}
		}
	}
	for line := range strings.SplitSeq(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 3 && trimmed[0] == '-' && trimmed[2] == ':' && trimmed[3] == ' ' {
			fmt.Fprintf(&flags, "  %s\n", trimmed[:2])
		}
	}
	fmt.Fprintf(&page, "Options:\n%s", flags.String())
	return page.String()
}
