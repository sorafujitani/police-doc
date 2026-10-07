package spec

import (
	"cmp"
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
	helpUsage          = regexp.MustCompile(`(?im)^[ \t]*usage(?:[ \t]*:|[ \t]*$)`)
	helpUsageHeading   = regexp.MustCompile(`(?im)^[ \t]*(?:usage|example usage|further help)(?:[ \t]*:|[ \t]*$)`)
	helpError          = regexp.MustCompile(`(?im)^\s*(?:(?:[^\r\n:]+: )?error:|fatal:|unknown command\b|unrecognized (?:command|argument)\b)`)
	helpColumns        = regexp.MustCompile(`\s{2,}`)
	helpCommandRow     = regexp.MustCompile(`^\s+([A-Za-z0-9][A-Za-z0-9_.:+-]*)\*?\s{2,}\S`)
	helpCommandLabel   = regexp.MustCompile(`^\s+([A-Za-z0-9][A-Za-z0-9_.:+-]*):$`)
	helpCommandHeading = regexp.MustCompile(`(?i)^(?:(?:available|common|management|all|other|additional|the)\s+)*(?:sub)?commands?(?:\s+are)?:?$|^these are .+\bcommands\b.*:$`)
	helpUpperHeading   = regexp.MustCompile(`^[A-Z][A-Z /-]*$`)
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

type helpReader func(context.Context, string, []string) (string, error)

// HelpCollector fills only requested, advertised paths. Its budget and failed
// attempts last for one scan; only successful help is stored in the snapshot.
type HelpCollector struct {
	Snapshot   *Snapshot
	Revision   int // Successful collections since loading the snapshot.
	options    HelpOptions
	depth      int
	calls      int
	aliasCalls int
	attempts   map[string]error
	readHelp   helpReader
}

func NewHelpCollector(ctx context.Context, options HelpOptions, snapshot *Snapshot) (*HelpCollector, error) {
	return newHelpCollector(ctx, options, snapshot, helpOutput)
}

func newHelpCollector(ctx context.Context, options HelpOptions, snapshot *Snapshot, readHelp helpReader) (*HelpCollector, error) {
	if options.Binary == "" {
		return nil, fmt.Errorf("help collection requires an executable")
	}
	options.Tool = cmp.Or(options.Tool, strings.TrimSuffix(filepath.Base(options.Binary), ".exe"))
	if !helpCommandName.MatchString(options.Tool) {
		return nil, fmt.Errorf("invalid tool name")
	}
	depth := DefaultHelpDepth
	if options.MaxDepth != nil {
		depth = *options.MaxDepth
	}
	options.MaxCommands = cmp.Or(options.MaxCommands, DefaultHelpCommands)
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
	collector := &HelpCollector{Snapshot: snapshot, options: options, depth: depth, attempts: make(map[string]error), readHelp: readHelp}
	if err := collector.Collect(ctx, nil); err != nil {
		return nil, err
	}
	return collector, nil
}

// Collect never turns arbitrary document arguments into executable requests.
// Every path component must already be advertised by its collected parent.
func (c *HelpCollector) Collect(ctx context.Context, path []string) error {
	return c.collect(ctx, path, &c.calls)
}

func (c *HelpCollector) collect(ctx context.Context, path []string, calls *int) (err error) {
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
	saveAttempt := true
	defer func() {
		if saveAttempt {
			c.attempts[key] = err
		}
	}()
	for _, args := range helpRequests(c.options.Binary, path) {
		if *calls >= c.options.MaxCommands {
			saveAttempt = false // Exhausting alias discovery must not poison normal collection.
			return fmt.Errorf("help call limit %d reached for %s", c.options.MaxCommands, key)
		}
		*calls++
		output, callErr := c.readHelp(ctx, c.options.Binary, args)
		if callErr != nil {
			err = callErr
			if ctx.Err() != nil {
				return err
			}
			continue
		}
		helpName := cmp.Or(c.Snapshot.Root.UsageName, c.options.Tool)
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

type usageInfo struct {
	matched, needsDetails                  bool
	name                                   string
	subcommandFirst, flagsAfterPositionals bool
}

// Usage alone is valid evidence for a command without flags or subcommands.
// Require the requested path so banners, empty headings and unrelated help
// do not turn a failed request into a successfully collected command.
func helpUsageInfo(output, tool string, path []string) usageInfo {
	matched, simple := false, false
	usageName := ""
	subcommandFirst, flagsAfterPositionals := true, true
	for _, words := range helpSynopses(output) {
		if len(words) < len(path)+1 {
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
		subcommandFirst = subcommandFirst && helpStartsWithSubcommand(words[len(path)+1:])
		flagsAfterPositionals = flagsAfterPositionals && helpFlagsAfterPositionals(words[len(path)+1:])
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
		simple = simple || !details
	}
	return usageInfo{
		matched: matched, needsDetails: matched && !simple, name: usageName,
		subcommandFirst: matched && subcommandFirst, flagsAfterPositionals: matched && flagsAfterPositionals,
	}
}

// Only an explicit command placeholder before any positional argument supports
// a warning about an unlisted subcommand. Ambiguous usage remains unchecked.
func helpStartsWithSubcommand(words []string) bool {
	for _, item := range helpUsageItems(words) {
		if helpCommandItem(item) {
			return true
		}
		if !helpOptionsItem(item) {
			return false
		}
	}
	return false
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
	choiceConflicts := make(map[string]bool)
	addFlag := func(name, value string, count int, choices []string) {
		if index, ok := flags[name]; ok {
			previous := &command.Flags[index]
			// Explicit value markers take precedence over a repeated bare heading.
			if !conflicts[name] && value != "unknown" && (value != "none" || previous.Value == "unknown") {
				if previous.Value != "unknown" && previous.Value != "none" && (previous.Value != value || previous.ValueCount != count) {
					previous.Value, previous.ValueCount, conflicts[name] = "unknown", 0, true
					previous.Choices, choiceConflicts[name] = nil, true
				} else {
					previous.Value, previous.ValueCount = value, count
				}
			}
			if previous.ValueCount > 1 {
				previous.Choices = nil
			} else if !choiceConflicts[name] && len(choices) > 0 {
				// Reordering the same choices is not a conflicting definition.
				if len(previous.Choices) > 0 && !sameChoices(previous.Choices, choices) {
					previous.Choices, choiceConflicts[name] = nil, true
				} else {
					previous.Choices = choices
				}
			}
			return
		}
		flags[name] = len(command.Flags)
		command.Flags = append(command.Flags, Flag{Name: name, Value: value, ValueCount: count, Choices: choices})
	}
	commands, usage := false, false
	commandIndent := -1
	output = strings.ReplaceAll(output, "\t", "    ")
	lines := strings.Split(output, "\n")
	flagLines := helpFlagLines(lines)
	for lineIndex, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" {
			usage = false
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if commands && commandIndent >= 0 && indent > commandIndent {
			continue // Nested descriptions cannot end the command catalog.
		}
		if commands && (commandIndent < 0 || indent == commandIndent) {
			if label := helpCommandLabel.FindStringSubmatch(line); label != nil {
				commandIndent = indent
				helpChild(&command, label[1])
				continue
			}
			if row := helpCommandRow.FindStringSubmatch(line); row != nil {
				commandIndent = indent
				helpChild(&command, strings.TrimSuffix(row[1], ":"))
				continue // A description ending in ':' is not a section heading.
			}
		}
		commandHeading := helpCommandHeading.MatchString(trimmed)
		upperHeading := indent == 0 && helpUpperHeading.MatchString(strings.TrimSuffix(trimmed, ":"))
		if commandHeading || strings.HasSuffix(lower, ":") || upperHeading {
			commands = commandHeading || (upperHeading && strings.HasSuffix(strings.TrimSuffix(trimmed, ":"), " COMMANDS"))
			commandIndent = -1
		}
		if commands && (commandIndent < 0 || indent == commandIndent) {
			if row := helpCommandRow.FindStringSubmatch(line); row != nil {
				commandIndent = indent
				helpChild(&command, strings.TrimSuffix(row[1], ":"))
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
		if !flagLines[lineIndex] || (!strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "[-")) {
			continue
		}
		// Read signatures, not flags mentioned in descriptions or examples.
		columns := helpColumns.Split(trimmed, 2)
		signature, description := columns[0], ""
		if len(columns) > 1 {
			description = columns[1]
		}
		var matches [][]int
		for _, match := range helpFlagName.FindAllStringIndex(signature, -1) {
			prefix := signature[:match[0]]
			if match[0] > 0 && !strings.ContainsRune(" [(,|/", rune(signature[match[0]-1])) {
				continue
			}
			if strings.LastIndex(prefix, "<") > strings.LastIndex(prefix, ">") || strings.LastIndex(prefix, "{") > strings.LastIndex(prefix, "}") {
				continue
			}
			matches = append(matches, match)
		}
		values := make([]string, len(matches))
		counts := make([]int, len(matches))
		choices := make([][]string, len(matches))
		aliasesOnly := true
		for i := 1; i < len(matches); i++ {
			aliasesOnly = aliasesOnly && helpAliasGap.MatchString(signature[matches[i-1][1]:matches[i][0]])
		}
		if !aliasesOnly {
			description = "" // One usage row may document unrelated flags.
		}
		for i, match := range matches {
			end := len(signature)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			tail := signature[match[1]:end]
			values[i], counts[i] = helpValueArity(tail)
			if counts[i] == 0 {
				// Multiple slots may have different choices; do not apply one set to all.
				choices[i] = helpFlagChoices(signature[match[0]:match[1]], tail, description)
			}
		}
		for i := len(matches) - 2; i >= 0; i-- {
			if helpAliasGap.MatchString(signature[matches[i][1]:matches[i+1][0]]) {
				values[i], counts[i], choices[i] = values[i+1], counts[i+1], choices[i+1]
				short := strings.TrimSuffix(signature[matches[i][0]:matches[i][1]], "...")
				long := signature[matches[i+1][0]:matches[i+1][1]]
				if len(short) == 2 && strings.HasPrefix(long, "--") {
					command.ShortFlagClusters = true // getopt-style short/long aliases.
				}
			}
		}
		for i, match := range matches {
			name := strings.TrimSuffix(signature[match[0]:match[1]], "...")
			if strings.Contains(name, "[no-]") {
				addFlag(strings.Replace(name, "[no-]", "", 1), values[i], counts[i], choices[i])
				addFlag(strings.Replace(name, "[no-]", "no-", 1), "unknown", 0, nil)
			} else {
				addFlag(name, values[i], counts[i], choices[i])
			}
		}
	}
	for _, flag := range command.Flags {
		if strings.HasPrefix(flag.Name, "-") && !strings.HasPrefix(flag.Name, "--") && len(flag.Name) > 2 {
			command.ShortFlagClusters = false // Go-style single-dash names do not imply clusters.
		}
	}
	return command
}

func helpValue(tail string) string {
	if strings.Trim(tail, " \t[],|/.") == "" {
		return "none"
	}
	if strings.HasPrefix(tail, "[=") {
		return "optional-attached"
	}
	if strings.HasPrefix(tail, "[<") {
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
	if (strings.HasPrefix(word, "<") && strings.Contains(word, ">")) || (strings.HasPrefix(word, "{") && strings.Contains(tail, "}")) {
		return "required"
	}
	if helpMetavar.MatchString(word) {
		return "required"
	}
	switch word {
	case "string", "strings", "stringArray", "int", "uint", "float", "duration", "list":
		return "required"
	}
	// Unrecognized value labels remain unknown; do not consume following words.
	return "unknown"
}
