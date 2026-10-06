package spec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSnapshotValidation(t *testing.T) {
	fixture := func() *Snapshot {
		return &Snapshot{Tool: "acme", Version: "1.0.0", Root: Command{
			Flags:    []Flag{{Name: "--output", Value: "required"}},
			Commands: []Command{{Name: "run"}},
		}}
	}
	if err := fixture().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"version", func(s *Snapshot) { s.Version = "latest" }},
		{"tool", func(s *Snapshot) { s.Tool = "../acme" }},
		{"root-name", func(s *Snapshot) { s.Root.Name = "acme" }},
		{"help-name", func(s *Snapshot) { s.Root.UsageName = "../acme" }},
		{"arity", func(s *Snapshot) { s.Root.Flags[0].Value = "invalid" }},
		{"bad-flag", func(s *Snapshot) { s.Root.Flags[0].Name = "--" }},
		{"duplicate-flag", func(s *Snapshot) { s.Root.Flags = append(s.Root.Flags, s.Root.Flags[0]) }},
		{"bad-command", func(s *Snapshot) { s.Root.Commands[0].Name = "--run" }},
		{"duplicate-command", func(s *Snapshot) { s.Root.Commands = append(s.Root.Commands, s.Root.Commands[0]) }},
		{"source-kind", func(s *Snapshot) { s.Sources = []Evidence{{Kind: "contract", Reference: "spec.json"}} }},
		{"empty-source", func(s *Snapshot) { s.Root.Sources = []Evidence{{Kind: "help"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := fixture()
			tc.mutate(snapshot)
			if err := snapshot.Validate(); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

func TestGoHelpUsesOnlyFixedCalls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("The fake Go binary uses a POSIX shell script.")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "go")
	log := filepath.Join(dir, "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	script := `#!/bin/sh
[ "$GOTOOLCHAIN" = local ] && [ "$GOENV" = off ] && [ "$GOWORK" = off ] || exit 10
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  version) printf 'go version go1.27.1 linux/amd64\n' ;;
  help) printf 'The commands are:\n\tbuild compile packages\n\ttest test packages\nAdditional help topics:\n\tmodules module support\n' ;;
  'help build') printf 'usage: go build [-o output] [build flags] [packages]\n\t-v\n\t-race\n' ;;
  *) exit 11 ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot := collectTestPaths(t, HelpOptions{Binary: binary}, "build")
	data, err := os.ReadFile(log)
	if err != nil || string(data) != "version\nhelp\nhelp build\n" {
		t.Fatalf("unexpected process calls: %q %v", data, err)
	}
	if snapshot.Version != "1.27.1" || snapshot.OS != runtime.GOOS || snapshot.Root.Child("modules") != nil {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	build := snapshot.Root.Child("build")
	if len(build.Flags) != 3 || build.Flags[0].Value != "required" || build.Flags[1].Value != "none" {
		t.Fatalf("documented value rules were not preserved: %+v", build)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewHelpCollector(ctx, HelpOptions{Binary: binary}, nil); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestBoundedHelpOutput(t *testing.T) {
	var output boundedOutput
	if _, err := output.Write([]byte(strings.Repeat("a", 1<<20))); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("b")); err == nil {
		t.Fatal("unbounded help output accepted")
	}
}
