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
	helpUsageHeading   = regexp.MustCompile(`(?im)^\s*(?:usage|example usage|further help)\s*:`)
	helpError          = regexp.MustCompile(`(?im)^\s*(?:(?:[^\r\n:]+: )?error:|fatal:|unknown command\b|unrecognized (?:command|argument)\b)`)
	helpANSI           = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
	helpColumns        = regexp.MustCompile(`\s{2,}`)
	helpCommandRow     = regexp.MustCompile(`^\s+([A-Za-z0-9][A-Za-z0-9_.:+-]*)\*?\s{2,}\S`)
	helpCommandLabel   = regexp.MustCompile(`^\s+([A-Za-z0-9][A-Za-z0-9_.:+-]*):$`)
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

// HelpCollector fills only requested, advertised paths. Its budget and failed
// attempts last for one scan; only successful help is stored in the snapshot.
type HelpCollector struct {
	Snapshot *Snapshot
	Revision int // Successful collections since loading the snapshot.
	options  HelpOptions
	depth    int
	calls    int
	attempts map[string]error
}

func NewHelpCollector(ctx context.Context, options HelpOptions, snapshot *Snapshot) (*HelpCollector, error) {
	if options.Binary == "" {
		return nil, fmt.Errorf("help collection requires an executable")
	}
	if options.Tool == "" {
		options.Tool = strings.TrimSuffix(filepath.Base(options.Binary), ".exe")
	}
	if !helpCommandName.MatchString(options.Tool) {
		return nil, fmt.Errorf("invalid tool name")
	}
	depth := DefaultHelpDepth
	if options.MaxDepth != nil {
		depth = *options.MaxDepth
	}
	if options.MaxCommands == 0 {
		options.MaxCommands = DefaultHelpCommands
	}
	if depth < 0 || options.MaxCommands < 1 {
		return nil, fmt.Errorf("help depth must be nonnegative and call limit must be positive")
	}
	if snapshot == nil {
		snapshot = &Snapshot{Tool: options.Tool, Version: options.Version, OS: runtime.GOOS}
		if options.Version == "" {
			version, evidence, err := DetectVersion(ctx, options.Binary)
			if err != nil {
				return nil, err
			}
			snapshot.Version = version
			snapshot.Sources = []Evidence{evidence}
		}
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	if snapshot.Tool != options.Tool || snapshot.OS != runtime.GOOS || (options.Version != "" && snapshot.Version != options.Version) {
		return nil, fmt.Errorf("help snapshot does not match the requested executable")
	}
	collector := &HelpCollector{Snapshot: snapshot, options: options, depth: depth, attempts: make(map[string]error)}
	if err := collector.Collect(ctx, nil); err != nil {
		return nil, err
	}
	return collector, nil
}

// Collect never turns arbitrary document arguments into executable requests.
// Every path component must already be advertised by its collected parent.
func (c *HelpCollector) Collect(ctx context.Context, path []string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := &c.Snapshot.Root
	for _, name := range path {
		if !helpCommandName.MatchString(name) || len(target.Sources) == 0 || target.Child(name) == nil {
			return fmt.Errorf("command path is not advertised by collected help")
		}
		target = target.Child(name)
	}
	if len(path) > c.depth {
		return fmt.Errorf("help depth limit %d reached for %s", c.depth, strings.Join(path, " "))
	}
	if len(target.Sources) > 0 {
		return nil
	}
	key := strings.Join(path, " ")
	if previous, ok := c.attempts[key]; ok {
		return previous
	}
	defer func() { c.attempts[key] = err }()
	for _, args := range helpRequests(c.options.Binary, path) {
		if c.calls >= c.options.MaxCommands {
			return fmt.Errorf("help call limit %d reached for %s", c.options.MaxCommands, key)
		}
		c.calls++
		output, callErr := helpOutput(ctx, c.options.Binary, args)
		if callErr != nil {
			err = callErr
			if ctx.Err() != nil {
				return err
			}
			continue
		}
		helpName := c.Snapshot.Root.UsageName
		if helpName == "" {
			helpName = c.options.Tool
		}
		parsed, parseErr := parseHelpPage(output, c.options.Binary, helpName, path)
		reference := strings.Join(append([]string{c.options.Binary}, args...), " ")
		if parseErr != nil {
			err = fmt.Errorf("%s: %w", reference, parseErr)
			continue
		}
		evidence := Evidence{Kind: "help", Reference: reference, Detail: "Help may omit flags, value rules and commands."}
		parsed.Name, parsed.Sources = target.Name, []Evidence{evidence}
		if err := validateCommand(&parsed, c.Snapshot.Tool+" "+key); err != nil {
			return err
		}
		*target = parsed
		c.Snapshot.Sources = append(c.Snapshot.Sources, evidence)
		c.Revision++
		return nil
	}
	return err
}

func helpChild(command *Command, name string) *Command {
	if child := command.Child(name); child != nil {
		return child
	}
	command.Commands = append(command.Commands, Command{Name: name})
	return &command.Commands[len(command.Commands)-1]
}

// Usage alone is valid evidence for a command without flags or subcommands.
// Require the requested path so banners, empty headings and unrelated help
// do not turn a failed request into a successfully collected command.
func helpUsageInfo(output, tool string, path []string) (matched, needsDetails bool, usageName string) {
	output = helpANSI.ReplaceAllString(output, "")
	primary := helpUsage.MatchString(output)
	usage, body := false, false
	for line := range strings.SplitSeq(output, "\n") {
		if match := helpUsage.FindStringIndex(line); match != nil {
			usage, body = true, false
			line = line[match[1]:]
		} else {
			trimmed := strings.ToLower(strings.TrimSpace(line))
			if trimmed == "" {
				if body {
					usage = false
				}
				continue
			}
			if strings.HasSuffix(trimmed, ":") {
				usage = !primary && (trimmed == "example usage:" || trimmed == "further help:")
				body = false
				continue
			}
		}
		words := strings.Fields(line)
		body = body || len(words) > 0
		if !usage || len(words) < len(path)+1 {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(words[0]), ".exe")
		if !helpCommandName.MatchString(name) || strings.HasSuffix(name, ":") || (tool != "" && name != tool) {
			continue
		}
		matches := true
		for i, name := range path {
			if !slices.Contains(strings.Split(strings.Trim(words[i+1], "[]():"), "|"), name) {
				matches = false
			}
		}
		if !matches || !helpUsageTail(words[len(path)+1:]) {
			continue
		}
		matched, usageName = true, name
		details := false
		for _, word := range words[len(path)+1:] {
			switch strings.ToLower(strings.Trim(word, "[]<>()")) {
			case "options", "flags":
				details = true
			case "command", "commands", "subcommand", "subcommands":
				// An advertised command may take another command as an argument.
				// A root placeholder alone provides no command-list evidence.
				details = details || len(path) == 0
			}
		}
		if !details {
			return true, false, usageName
		}
	}
	return matched, matched, usageName
}

// Bare lowercase words in a synopsis can be deeper command names. Do not
// assign that page's flags to the requested parent. Bracketed groups and
// explicit metavariables describe arguments rather than a fixed command path.
func helpUsageTail(words []string) bool {
	depth := 0
	for _, word := range words {
		if depth == 0 && helpCommandName.MatchString(word) && !helpMetavar.MatchString(word) && word != "..." {
			return false
		}
		for _, char := range word {
			switch char {
			case '[', '<', '(':
				depth++
			case ']', '>', ')':
				depth = max(0, depth-1)
			}
		}
	}
	return true
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
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if commands && (commandIndent < 0 || indent == commandIndent) {
			if label := helpCommandLabel.FindStringSubmatch(line); label != nil {
				commandIndent = indent
				helpChild(&command, label[1])
				continue
			}
		}
		if strings.HasSuffix(lower, ":") {
			commands = helpCommandHeading.MatchString(lower)
			commandIndent = -1
		}
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
