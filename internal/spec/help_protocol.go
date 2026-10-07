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
	parsed.Aliases = helpAliases(output, usage.name, path)
	// Some root pages (notably npm) show example invocations instead of a
	// root synopsis. Their explicit command catalog still identifies the root.
	if helpUsageHeading.MatchString(output) && !usage.matched && (len(path) > 0 || len(parsed.Commands) == 0) {
		return Command{}, fmt.Errorf("help Usage does not describe the requested command unambiguously: %q", strings.Join(append([]string{tool}, path...), " "))
	}
	if len(parsed.Flags) == 0 && len(parsed.Commands) == 0 && (!usage.matched || usage.needsDetails) {
		return Command{}, fmt.Errorf("could not recognize help usage, flags or commands")
	}
	return parsed, nil
}
