package spec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var goFlagLine = regexp.MustCompile(`^\t(-[A-Za-z][A-Za-z0-9-]*)(?:[ =](.+))?$`)

// collectGo uses fixed help calls and never downloads a toolchain.
func collectGo(ctx context.Context, binary string, snapshot *Snapshot) error {
	rootHelp, err := goOutput(ctx, binary, "help")
	if err != nil {
		return err
	}
	rootSource := Evidence{Kind: "help", Reference: binary + " help"}
	snapshot.Sources = append(snapshot.Sources, rootSource)
	snapshot.Root.Sources = []Evidence{rootSource}
	reading := false
	for line := range strings.SplitSeq(rootHelp, "\n") {
		if strings.TrimSpace(line) == "The commands are:" {
			reading = true
			continue
		}
		if reading && strings.TrimSpace(line) == "Additional help topics:" {
			break
		}
		if reading && strings.HasPrefix(line, "\t") {
			words := strings.Fields(line)
			if len(words) > 1 && validName(words[0]) {
				snapshot.Root.Commands = append(snapshot.Root.Commands, Command{Name: words[0]})
			}
		}
	}
	if len(snapshot.Root.Commands) == 0 {
		return fmt.Errorf("could not recognize the go help command list")
	}
	build := snapshot.Root.Child("build")
	if build == nil {
		return fmt.Errorf("go help does not list build")
	}
	buildHelp, err := goOutput(ctx, binary, "help", "build")
	if err != nil {
		return err
	}
	build.Sources = []Evidence{{Kind: "help", Reference: binary + " help build", Detail: "Usage and flag headings collected; hidden flags and argument rules may be missing."}}
	// Go documents -o in the usage line rather than in its flag headings.
	hasOutput := strings.HasPrefix(buildHelp, "usage: go build [-o output]")
	if hasOutput {
		build.Flags = append(build.Flags, Flag{Name: "-o", Value: "required"})
	}
	for line := range strings.SplitSeq(buildHelp, "\n") {
		match := goFlagLine.FindStringSubmatch(line)
		if match == nil || (hasOutput && match[1] == "-o") {
			continue
		}
		value := "unknown"
		if strings.TrimSpace(match[2]) != "" {
			value = "required"
		}
		build.Flags = append(build.Flags, Flag{Name: match[1], Value: value})
	}
	if len(build.Flags) == 0 {
		return fmt.Errorf("could not recognize go help build flag headings")
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	return nil
}

type boundedOutput struct {
	// Do not embed Buffer: its ReadFrom would let io.Copy bypass Write's limit.
	buffer   bytes.Buffer
	exceeded bool
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	if output.buffer.Len()+len(data) > 1<<20 {
		output.exceeded = true
		return 0, fmt.Errorf("help output exceeds 1 MiB")
	}
	return output.buffer.Write(data)
}

func goOutput(ctx context.Context, binary string, args ...string) (string, error) {
	// Avoid automatic toolchain downloads, including when invoked in a module
	// whose go directive is newer than this installed Go binary.
	return commandOutput(ctx, binary, []string{"GOTOOLCHAIN=local", "GOENV=off", "GOWORK=off"}, false, args...)
}

func commandOutput(ctx context.Context, binary string, env []string, help bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = append(os.Environ(), env...)
	command.WaitDelay = time.Second
	var output boundedOutput
	// A shared writer preserves both streams and enforces one combined limit.
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	if output.exceeded {
		return "", fmt.Errorf("collecting %s: help output exceeds 1 MiB", binary)
	}
	if ctx.Err() != nil {
		return "", fmt.Errorf("collecting %s: %w", binary, ctx.Err())
	}
	text := output.buffer.String()
	if help && helpError.MatchString(text) {
		return "", fmt.Errorf("collecting %s %s: CLI reported an error: %s", binary, strings.Join(args, " "), strings.TrimSpace(text))
	}
	if err != nil {
		// Some CLIs (notably git -h) print normal usage and exit nonzero.
		if exit, ok := errors.AsType[*exec.ExitError](err); help && ok && helpUsage.MatchString(text) && (exit.ExitCode() == 1 || exit.ExitCode() == 2 || exit.ExitCode() == 129) {
			return text, nil
		}
		return "", fmt.Errorf("collecting %s %s: %w: %s", binary, strings.Join(args, " "), err, strings.TrimSpace(text))
	}
	return text, nil
}
