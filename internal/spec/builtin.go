package spec

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

var builtinSwitchGroup = regexp.MustCompile(`\[-([A-Za-z]+(?:\|-[A-Za-z]+)*)\]`)

// CollectBuiltinHelp queries Bash's help builtin, not an external executable
// with the builtin's name. Privileged, noninteractive mode suppresses startup
// files, imported functions, BASH_ENV, ENV, and inherited shell option settings.
// The fixed program receives the name as a quoted argument, never shell code.
func CollectBuiltinHelp(ctx context.Context, binary, name string) (*Snapshot, error) {
	if !validName(name) {
		return nil, fmt.Errorf("invalid builtin name")
	}
	version, evidence, err := DetectVersion(ctx, binary)
	if err != nil {
		return nil, err
	}
	output, err := commandOutput(ctx, binary, []string{"LC_ALL=C", "BASH_ENV=", "ENV=", "PAGER=cat"}, true,
		"--noprofile", "--norc", "-p", "-c", `builtin help -- "$1"`, "policedoc-metadata", name)
	if err != nil {
		return nil, err
	}
	first, _, _ := strings.Cut(output, "\n")
	usage, ok := strings.CutPrefix(first, name+": ")
	if !ok {
		return nil, fmt.Errorf("Bash did not identify builtin %q", name)
	}
	var switches []Flag
	usage = builtinSwitchGroup.ReplaceAllStringFunc(usage, func(group string) string {
		for _, r := range strings.Trim(group, "[]") {
			if r == '-' || r == '|' {
				continue
			}
			switches = append(switches, Flag{Name: "-" + string(r), Value: "none"})
		}
		return "[OPTIONS]"
	})
	// The shell's own synopsis supplies shape, but not an exhaustive language
	// for eval, test, sourced files, or user-defined functions.
	command := parseHelp("Usage: " + usage + "\n" + output)
	for _, flag := range switches {
		if command.Flag(flag.Name) == nil {
			command.Flags = append(command.Flags, flag)
		}
	}
	command.UsageName, command.ShortFlagClusters, command.FlagOrderUnknown = name, true, true
	command.Forms = synopsisForms("Usage: "+usage, name, nil, &command)
	command.DerivedForms = len(command.Forms) > 0
	// These builtins accept every string argv. Their options affect output or
	// status, not syntactic acceptance; unknown dash-prefixed words are data.
	if name == "echo" || name == "true" || name == "false" {
		command.Flags, command.LiteralArguments = nil, true
		command.Forms, command.DerivedForms = []Form{{MinArgs: 0}}, false
	}
	source := Evidence{Kind: "help", Reference: binary + " builtin help " + name, Detail: "Bash builtin metadata only; startup files and imported functions were disabled."}
	command.Sources = []Evidence{source}
	snapshot := &Snapshot{Tool: name, Shell: "bash", Version: version, OS: runtime.GOOS, Sources: []Evidence{evidence, source}, Root: command}
	return snapshot, snapshot.Validate()
}
