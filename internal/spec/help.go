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
