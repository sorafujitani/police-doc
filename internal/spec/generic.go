package spec

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

const (
	DefaultHelpDepth    = 3
	DefaultHelpCommands = 100
)

// HelpOptions selects a trusted executable, never document arguments.
type HelpOptions struct {
	Binary      string
	Tool        string
	Version     string
	MaxDepth    *int // nil uses DefaultHelpDepth; zero disables discovery.
	MaxCommands int  // Maximum help calls, including root; zero uses DefaultHelpCommands.
}

var (
	helpEnvironment    = []string{"LC_ALL=C", "LANG=C", "NO_COLOR=1", "CLICOLOR=0", "PAGER=cat", "GIT_PAGER=cat"}
	helpCommandName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:+-]*$`)
	helpVersion        = regexp.MustCompile(`\bv?[0-9]+\.[0-9]+(?:\.[0-9A-Za-z]+)*(?:[-+][0-9A-Za-z.+-]+)?\b`)
	helpUsage          = regexp.MustCompile(`(?im)^\s*usage\s*:`)
	helpError          = regexp.MustCompile(`(?im)^\s*(?:(?:[^\r\n:]+: )?error:|fatal:|unknown command\b|unrecognized (?:command|argument)\b)`)
	helpANSI           = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
	helpColumns        = regexp.MustCompile(`\s{2,}`)
	helpCommandRow     = regexp.MustCompile(`^\s+([A-Za-z0-9][A-Za-z0-9_.:+-]*)\*?\s{2,}\S`)
	helpCommandHeading = regexp.MustCompile(`(?i)^(?:(?:available|common|management|all|other|additional|the)\s+)*(?:sub)?commands?(?:\s+are)?:$|^these are .+\bcommands\b.*:$`)
	helpFlagName       = regexp.MustCompile(`--(?:\[no-\])?[A-Za-z0-9][A-Za-z0-9_.:-]*|-[A-Za-z0-9][A-Za-z0-9_.:-]*`)
	helpAliasGap       = regexp.MustCompile(`^\s*[,|/]\s*$`)
	helpMetavar        = regexp.MustCompile(`^[A-Z][A-Z0-9_]*(?:\.\.\.)?$`)
)

// DetectVersion checks the installed CLI without collecting its command tree.
func DetectVersion(ctx context.Context, binary string) (string, Evidence, error) {
	reference := binary + " --version"
	var output string
	var err error
	if isGo(binary) {
		output, err = goOutput(ctx, binary, "version")
		reference = binary + " version"
	} else {
		output, err = commandOutput(ctx, binary, helpEnvironment, false, "--version")
	}
	if err != nil {
		return "", Evidence{}, err
	}
	firstLine, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	versionLine := firstLine
	if isGo(binary) {
		fields := strings.Fields(firstLine)
		if len(fields) != 4 || fields[0] != "go" || fields[1] != "version" || !strings.HasPrefix(fields[2], "go") || !strings.Contains(fields[3], "/") {
			return "", Evidence{}, fmt.Errorf("unexpected go version output")
		}
		versionLine = strings.TrimPrefix(fields[2], "go")
	}
	versions := helpVersion.FindAllString(versionLine, -1)
	if len(versions) != 1 {
		return "", Evidence{}, fmt.Errorf("could not recognize a single CLI version")
	}
	return strings.TrimPrefix(versions[0], "v"), Evidence{Kind: "help", Reference: reference, Detail: firstLine}, nil
}

func isGo(binary string) bool {
	return strings.TrimSuffix(filepath.Base(binary), ".exe") == "go"
}

// CollectHelp follows advertised commands within bounded safety limits.
// A failed child stays a placeholder; failed root help aborts collection.
func CollectHelp(ctx context.Context, options HelpOptions) (*Snapshot, []string, error) {
	if options.Binary == "" {
		return nil, nil, fmt.Errorf("help collection requires an executable")
	}
	if options.Tool == "" {
		options.Tool = strings.TrimSuffix(filepath.Base(options.Binary), ".exe")
	}
	if !helpCommandName.MatchString(options.Tool) {
		return nil, nil, fmt.Errorf("invalid tool name")
	}
	maxDepth := DefaultHelpDepth
	if options.MaxDepth != nil {
		maxDepth = *options.MaxDepth
	}
	if options.MaxCommands == 0 {
		options.MaxCommands = DefaultHelpCommands
	}
	if maxDepth < 0 || options.MaxCommands < 1 {
		return nil, nil, fmt.Errorf("help depth must be nonnegative and call limit must be positive")
	}
	type request struct {
		path  []string
		depth int
	}
	requests := []request{{}}
	seen := map[string]bool{"": true}
	snapshot := &Snapshot{Tool: options.Tool, Version: options.Version, OS: runtime.GOOS}
	if options.Version == "" {
		version, evidence, err := DetectVersion(ctx, options.Binary)
		if err != nil {
			return nil, nil, err
		}
		snapshot.Version = version
		snapshot.Sources = append(snapshot.Sources, evidence)
	}
	if err := snapshot.Validate(); err != nil {
		return nil, nil, err
	}
	if isGo(options.Binary) {
		if err := collectGo(ctx, options.Binary, snapshot); err != nil {
			return nil, nil, err
		}
		return snapshot, nil, nil
	}
	var warnings []string
	commandLimit := false
	calls := 0
	for index := 0; index < len(requests); index++ {
		current := requests[index]
		if calls >= options.MaxCommands {
			commandLimit = true
			break
		}
		var parsed Command
		var reference string
		var err error
		for _, helpFlag := range []string{"-h", "--help"} {
			if calls >= options.MaxCommands {
				commandLimit = true
				break
			}
			args := append(slices.Clone(current.path), helpFlag)
			calls++
			var output string
			output, err = commandOutput(ctx, options.Binary, helpEnvironment, true, args...)
			parsed = parseHelp(output)
			reference = strings.Join(append([]string{options.Binary}, args...), " ")
			if err == nil && len(parsed.Flags) == 0 && len(parsed.Commands) == 0 {
				err = fmt.Errorf("%s: could not recognize help flags or commands", reference)
			}
			if err == nil || ctx.Err() != nil {
				break
			}
		}
		if err != nil {
			if index == 0 || ctx.Err() != nil {
				return nil, warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("Help unavailable; keeping a placeholder: %.300s", err))
			continue
		}
		evidence := Evidence{Kind: "help", Reference: reference, Detail: "Help may omit flags, value rules and commands."}
		snapshot.Sources = append(snapshot.Sources, evidence)
		target := &snapshot.Root
		for _, name := range current.path {
			target = helpChild(target, name)
		}
		target.Flags = parsed.Flags
		target.Sources = []Evidence{evidence}
		depthLimit := false
		for _, child := range parsed.Commands {
			helpChild(target, child.Name)
			path := append(slices.Clone(current.path), child.Name)
			key := strings.Join(path, " ")
			if seen[key] {
				continue
			}
			if current.depth >= maxDepth {
				depthLimit = true
				continue
			}
			if len(requests) >= options.MaxCommands {
				commandLimit = true
				continue
			}
			requests = append(requests, request{path: path, depth: current.depth + 1})
			seen[key] = true
		}
		if depthLimit {
			warnings = append(warnings, fmt.Sprintf("%s: help depth limit %d reached; deeper commands remain placeholders", reference, maxDepth))
		}
	}
	if commandLimit {
		warnings = append(warnings, fmt.Sprintf("help call limit %d reached; unvisited commands remain placeholders", options.MaxCommands))
	}
	if err := snapshot.Validate(); err != nil {
		return nil, warnings, err
	}
	return snapshot, warnings, nil
}

func helpChild(command *Command, name string) *Command {
	if child := command.Child(name); child != nil {
		return child
	}
	command.Commands = append(command.Commands, Command{Name: name})
	return &command.Commands[len(command.Commands)-1]
}

func parseHelp(output string) Command {
	var command Command
	flags := make(map[string]int)
	conflicts := make(map[string]bool)
	addFlag := func(name, value string) {
		if index, ok := flags[name]; ok {
			previous := &command.Flags[index]
			if conflicts[name] || value == "unknown" {
				return
			}
			if previous.Value != "unknown" && previous.Value != value {
				previous.Value, conflicts[name] = "unknown", true
			} else {
				previous.Value = value
			}
			return
		}
		flags[name] = len(command.Flags)
		command.Flags = append(command.Flags, Flag{Name: name, Value: value})
	}
	commands, usage := false, false
	commandIndent := -1
	output = strings.ReplaceAll(helpANSI.ReplaceAllString(output, ""), "\t", "    ")
	for line := range strings.SplitSeq(output, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" {
			usage = false
			continue
		}
		if strings.HasSuffix(lower, ":") {
			commands = helpCommandHeading.MatchString(lower)
			commandIndent = -1
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if commands && (commandIndent < 0 || indent == commandIndent) {
			if row := helpCommandRow.FindStringSubmatch(line); row != nil {
				commandIndent = indent
				helpChild(&command, row[1])
			} else if strings.HasPrefix(line, " ") {
				// Some help formats use comma-separated names instead of a table.
				names := strings.Split(trimmed, ",")
				valid := true
				for _, name := range names {
					if name = strings.TrimSpace(name); name != "" && !helpCommandName.MatchString(name) {
						valid = false
					}
				}
				if valid {
					commandIndent = indent
					for _, name := range names {
						if name = strings.TrimSpace(name); name != "" {
							helpChild(&command, name)
						}
					}
				}
			}
		}
		if helpUsage.MatchString(line) {
			usage = true
		}
		if usage && !strings.HasPrefix(trimmed, "-") {
			if start := strings.Index(trimmed, "["); start >= 0 {
				trimmed = trimmed[start:]
			}
		}
		if !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "[-") {
			continue
		}
		// Read signatures, not flags mentioned in descriptions or examples.
		signature := helpColumns.Split(trimmed, 2)[0]
		var matches [][]int
		for _, match := range helpFlagName.FindAllStringIndex(signature, -1) {
			prefix := signature[:match[0]]
			if match[0] > 0 && !strings.ContainsRune(" [(,|/", rune(signature[match[0]-1])) {
				continue
			}
			if strings.LastIndex(prefix, "<") > strings.LastIndex(prefix, ">") {
				continue
			}
			matches = append(matches, match)
		}
		values := make([]string, len(matches))
		for i, match := range matches {
			end := len(signature)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			values[i] = helpValue(signature[match[1]:end])
		}
		for i := len(matches) - 2; i >= 0; i-- {
			if helpAliasGap.MatchString(signature[matches[i][1]:matches[i+1][0]]) {
				values[i] = values[i+1]
			}
		}
		for i, match := range matches {
			name := strings.TrimSuffix(signature[match[0]:match[1]], "...")
			if strings.Contains(name, "[no-]") {
				addFlag(strings.Replace(name, "[no-]", "", 1), values[i])
				addFlag(strings.Replace(name, "[no-]", "no-", 1), "unknown")
			} else {
				addFlag(name, values[i])
			}
		}
	}
	return command
}

func helpValue(tail string) string {
	if strings.HasPrefix(tail, "[=") || strings.HasPrefix(tail, "[<") {
		return "optional"
	}
	if strings.HasPrefix(tail, "=") {
		return "required"
	}
	if !strings.HasPrefix(tail, " ") {
		return "unknown"
	}
	words := strings.Fields(tail)
	if len(words) == 0 {
		return "unknown"
	}
	word := strings.TrimRight(words[0], "],")
	if strings.HasPrefix(word, "<") && strings.Contains(word, ">") {
		return "required"
	}
	if helpMetavar.MatchString(word) {
		return "required"
	}
	switch word {
	case "string", "strings", "stringArray", "int", "uint", "float", "duration", "list":
		return "required"
	}
	// Unmarked arity stays unknown; descriptions are not proof of a value rule.
	return "unknown"
}
