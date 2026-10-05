package spec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseGenericHelp(t *testing.T) {
	for _, tc := range []struct {
		name, help string
		commands   []string
		flags      map[string]string
	}{
		{"git-root", `usage: git [-v | --version] [-C <path>]
           [--exec-path[=<path>]] <command> [<args>]

These are common Git commands used in various situations:

start a working area (see also: git help tutorial)
   clone      Clone a repository
   init       Create an empty repository
`, []string{"clone", "init"}, map[string]string{"-v": "unknown", "--version": "unknown", "-C": "required", "--exec-path": "optional"}},
		{"git-flags", `usage: git commit [options]
    -m, --[no-]message <message>  commit message
    -S, --[no-]gpg-sign[=<key-id>]
    --trailer <trailer-value>  add a trailer (see --not-a-flag)
`, nil, map[string]string{"-m": "required", "--message": "required", "--no-message": "unknown", "-S": "optional", "--gpg-sign": "optional", "--no-gpg-sign": "unknown", "--trailer": "required"}},
		{"npm-root", `npm <command>

All commands:
    install, run, test,
    version

Options:
    --help  Print help
`, []string{"install", "run", "test", "version"}, map[string]string{"--help": "unknown"}},
		{"npm-flags", `Usage:
npm install [<package-spec> ...]

Options:
[-E|--save-exact] [-g|--global]
[--omit <dev|optional|peer> [--omit <dev|optional|peer> ...]]
[-w|--workspace <workspace-name> [-w|--workspace <workspace-name> ...]]
  --omit
    Dependency types to omit.
`, nil, map[string]string{"-E": "unknown", "--save-exact": "unknown", "-g": "unknown", "--global": "unknown", "--omit": "required", "-w": "required", "--workspace": "required"}},
		{"docker", `Usage: docker [OPTIONS] COMMAND

Common Commands:
  run         Run a container
Management Commands:
  container   Manage containers
  compose*    Docker Compose
Commands:
  run         Run a container

Global Options:
      --config string      Config directory
  -H, --host list          Daemon sockets
      --debug              Enable debugging (see --not-a-flag)
`, []string{"run", "container", "compose"}, map[string]string{"--config": "required", "-H": "required", "--host": "required", "--debug": "unknown"}},
		{"other-formats", "Usage: acme [OPTIONS]\n\nOptions:\n\t-o, --output FILE\tOutput file\n  --color[=WHEN]  Color mode\n  --dry_run  Dry run\n  \x1b[32m--define\x1b[0m=<key=value>  Define value\n  --mode <--fast|--slow>  Modes, not flags\n", nil, map[string]string{"-o": "required", "--output": "required", "--color": "optional", "--dry_run": "unknown", "--define": "required", "--mode": "required"}},
		{"uv-repeated-flags", "Options:\n  -q, --quiet...\n  -v, --verbose...\n", nil, map[string]string{"-q": "unknown", "--quiet": "unknown", "-v": "unknown", "--verbose": "unknown"}},
		{"ripgrep-alias-values", "Options:\n    -e PATTERN, --regexp=PATTERN\n    -g GLOB, --glob=GLOB\n", nil, map[string]string{"-e": "required", "--regexp": "required", "-g": "required", "--glob": "required"}},
		{"conflicting-arity", "Options:\n  --output FILE\n  --output[=FILE]\n  --output FILE\n", nil, map[string]string{"--output": "unknown"}},
		{"examples-not-commands", `Usage: searcher [OPTIONS] PATTERN
Options:
    --pre COMMAND  Preprocessor
        For example, a shell script for COMMAND might look like:
            case "$1" in
                exec processor "$1"
            esac
        Example commands:
            searcher    Some documentation example
`, nil, map[string]string{"--pre": "required"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := parseHelp(tc.help)
			flags := make(map[string]string)
			for _, flag := range command.Flags {
				flags[flag.Name] = flag.Value
			}
			var commands []string
			for _, child := range command.Commands {
				commands = append(commands, child.Name)
			}
			if !reflect.DeepEqual(flags, tc.flags) || !reflect.DeepEqual(commands, tc.commands) {
				t.Fatalf("commands=%v flags=%v", commands, flags)
			}
			snapshot := Snapshot{Tool: "acme", Version: "1.0.0", Root: command}
			if err := snapshot.Validate(); err != nil {
				t.Fatalf("invalid snapshot: %+v, %v", command, err)
			}
		})
	}
}

func helpExecutable(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("The fake CLI uses a POSIX shell script.")
	}
	binary := filepath.Join(t.TempDir(), "acme")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary
}

func TestCollectHelpVersions(t *testing.T) {
	for _, tc := range []struct{ output, want string }{
		{"v24.21.0", "24.21.0"},
		{"acme version 1.2.3", "1.2.3"},
		{"acme v1.2.3-rc.1+build.7", "1.2.3-rc.1+build.7"},
	} {
		t.Run(tc.output, func(t *testing.T) {
			t.Setenv("POLICEDOC_TEST_VERSION", tc.output)
			binary := helpExecutable(t, `
if [ "$1" = --version ]; then
  printf '%s\n' "$POLICEDOC_TEST_VERSION"
else
  printf 'Usage: acme [options]\n  --help  Show help\n'
fi
`)
			contract, _, err := CollectHelp(context.Background(), HelpOptions{Binary: binary})
			if err != nil {
				t.Fatal(err)
			}
			if contract.Version != tc.want {
				t.Fatalf("version = %q, want %q", contract.Version, tc.want)
			}
		})
	}
}

func TestCollectHelpUsesOnlyAdvertisedPaths(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	binary := helpExecutable(t, `
[ "$LC_ALL" = C ] && [ "$NO_COLOR" = 1 ] && [ "$PAGER" = cat ] || exit 10
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) printf 'acme version 1.2.3-beta.1, build abc\n' >&2 ;;
  -h) printf 'Usage: acme [options]\nCommands:\n  container   Manage containers\n  bad;touch marker  Invalid command\n' ;;
  'container -h') printf 'Usage: acme container COMMAND\nCommands:\n  run  Run a container\n' ;;
  'container run -h') printf 'Usage: acme container run [options]\nOptions:\n  -o, --output <file>  Output file\n' >&2; exit 129 ;;
  *) exit 11 ;;
esac
`)
	snapshot, warnings, err := CollectHelp(context.Background(), HelpOptions{Binary: binary})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("collection failed: %v %v", err, warnings)
	}
	calls, err := os.ReadFile(log)
	if err != nil || string(calls) != "--version\n-h\ncontainer -h\ncontainer run -h\n" {
		t.Fatalf("unexpected calls: %q %v", calls, err)
	}
	run := snapshot.Root.Child("container").Child("run")
	if snapshot.Tool != "acme" || snapshot.Version != "1.2.3-beta.1" || snapshot.OS != runtime.GOOS || len(snapshot.Sources) != 4 || len(run.Flags) != 2 || run.Flags[0].Value != "required" || len(run.Sources) != 1 {
		t.Fatalf("unexpected snapshot: %+v; run=%+v", snapshot, run)
	}
}

func TestCollectHelpDiscoversChangingCommands(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	binary := helpExecutable(t, `
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) printf 'quartz 4.5.6\n' ;;
  -h) printf 'Usage: quartz COMMAND\nThese are common Quartz commands for local work:\n  %s     Dynamic command\n      continuation  Not another command\n  %s     Repeated listing\n  bad;touch marker  Invalid command name\n  ../escape  Invalid command name\n' "$POLICEDOC_TEST_CHILD" "$POLICEDOC_TEST_CHILD" ;;
  "$POLICEDOC_TEST_CHILD -h") printf 'Usage: quartz group COMMAND\nSubcommands:\n  leaf  Nested command\n' ;;
  "$POLICEDOC_TEST_CHILD leaf -h") printf 'Usage: quartz group leaf [OPTIONS]\nOptions:\n  --destination <file>  Output file\n' ;;
  *) exit 11 ;;
esac
`)
	for _, name := range []string{"fresh-command", "newly-added"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("POLICEDOC_TEST_CHILD", name)
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			snapshot, warnings, err := CollectHelp(context.Background(), HelpOptions{Binary: binary, Tool: "quartz"})
			if err != nil || len(warnings) != 0 {
				t.Fatalf("discovery failed: %v %v", err, warnings)
			}
			calls, err := os.ReadFile(log)
			want := "--version\n-h\n" + name + " -h\n" + name + " leaf -h\n"
			if err != nil || string(calls) != want {
				t.Fatalf("unexpected calls: %q %v", calls, err)
			}
			group := snapshot.Root.Child(name)
			if len(snapshot.Root.Commands) != 1 || group == nil || group.Child("leaf") == nil {
				t.Fatalf("dynamic command tree not collected: %+v", snapshot.Root)
			}
			leaf := group.Child("leaf")
			if len(leaf.Flags) != 1 || leaf.Flags[0].Name != "--destination" || leaf.Flags[0].Value != "required" || len(leaf.Sources) != 1 {
				t.Fatalf("nested flags not collected: %+v", leaf)
			}
		})
	}
}

func TestCollectHelpDiscoveryLimitsAndFailures(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	binary := helpExecutable(t, `
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  -h) printf 'Usage: acme COMMAND\nCommands:\n  branch       A group\n  unavailable  Broken plugin\n  empty        Unsupported help\n  recursive    Repeating help\n' ;;
  'branch -h') printf 'Usage: acme branch COMMAND\nCommands:\n  leaf  A leaf\n' ;;
  'branch leaf -h') printf 'Usage: acme branch leaf [OPTIONS]\n  --output FILE  Output file\n' ;;
  'unavailable -h') exit 3 ;;
  empty*) echo 'No supported help format' ;;
  recursive*) printf 'Usage: acme recursive COMMAND\nCommands:\n  recursive  Repeating help\n' ;;
  *) exit 11 ;;
esac
`)
	zero, one := 0, 1
	for _, tc := range []struct {
		name     string
		depth    *int
		limit    int
		calls    int
		warnings []string
	}{
		{"root-only", &zero, 100, 1, []string{"help depth limit"}},
		{"one-level", &one, 100, 7, []string{"help depth limit", "unavailable --help", "could not recognize help"}},
		{"call-limit", nil, 2, 2, []string{"help call limit"}},
		{"default-depth", nil, 100, 10, []string{"help depth limit", "unavailable --help", "could not recognize help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			snapshot, warnings, err := CollectHelp(context.Background(), HelpOptions{
				Binary: binary, Version: "1.0.0", MaxDepth: tc.depth, MaxCommands: tc.limit,
			})
			if err != nil {
				t.Fatal(err)
			}
			calls, err := os.ReadFile(log)
			if err != nil || len(strings.Split(strings.TrimSpace(string(calls)), "\n")) != tc.calls {
				t.Fatalf("unexpected calls: %q %v", calls, err)
			}
			for _, want := range tc.warnings {
				if !strings.Contains(strings.Join(warnings, "\n"), want) {
					t.Fatalf("missing warning %q: %v", want, warnings)
				}
			}
			if len(tc.warnings) == 0 && len(warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", warnings)
			}
			if snapshot.Root.Child("unavailable") == nil || len(snapshot.Root.Child("unavailable").Sources) != 0 {
				t.Fatalf("unavailable command must remain a placeholder: %+v", snapshot.Root)
			}
		})
	}
}

func TestCollectHelpCancellationDuringDiscovery(t *testing.T) {
	binary := helpExecutable(t, `
case "$*" in
  -h) printf 'Usage: acme COMMAND\nCommands:\n  slow  Slow help\n' ;;
  'slow -h') exec sleep 10 ;;
  *) exit 11 ;;
esac
`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	snapshot, _, err := CollectHelp(ctx, HelpOptions{Binary: binary, Version: "1.0.0"})
	if !errors.Is(err, context.DeadlineExceeded) || snapshot != nil {
		t.Fatalf("cancellation must abort, not become a discovery warning: %v", err)
	}
}

func TestCollectHelpRejectsInvalidOptionsBeforeExecution(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed")
	t.Setenv("POLICEDOC_TEST_MARKER", marker)
	binary := helpExecutable(t, `touch "$POLICEDOC_TEST_MARKER"; exit 1`)
	negative := -1
	for _, options := range []HelpOptions{
		{}, {Binary: binary, Tool: "bad/name"}, {Binary: binary, Version: "latest"},
		{Binary: binary, MaxDepth: &negative}, {Binary: binary, MaxCommands: -1},
	} {
		if _, _, err := CollectHelp(context.Background(), options); err == nil {
			t.Errorf("accepted %+v", options)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("invalid input executed the CLI: %v", err)
	}
}

func TestAutomaticHelpFlagAndCallBudget(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	binary := helpExecutable(t, `
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 1.2.3 ;;
  --help) printf 'Usage: acme COMMAND\nCommands:\n  next  A subcommand\n' ;;
  'next --help') printf 'Usage: acme next [OPTIONS]\n  --output FILE  Output file\n' ;;
  *) printf 'error: unknown flag\n'; exit 2 ;;
esac
`)
	for _, limit := range []int{1, 3, 4} {
		if err := os.WriteFile(log, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		snapshot, warnings, err := CollectHelp(context.Background(), HelpOptions{Binary: binary, MaxCommands: limit})
		calls, readErr := os.ReadFile(log)
		if readErr != nil || strings.Count(string(calls), "\n") != limit+1 {
			t.Fatalf("call budget %d violated: %q %v", limit, calls, readErr)
		}
		if limit == 1 {
			if err == nil || snapshot != nil {
				t.Fatal("root fallback exceeded its budget")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		child := snapshot.Root.Child("next")
		if limit == 3 && (len(warnings) == 0 || len(child.Sources) != 0) {
			t.Fatal("limited fallback did not leave a warned placeholder")
		}
		if limit == 4 && (len(warnings) != 0 || len(child.Flags) != 1 || !strings.HasSuffix(child.Sources[0].Reference, "next --help")) {
			t.Fatalf("fallback failed: %+v %v", child, warnings)
		}
	}
}

func TestCollectHelpFailures(t *testing.T) {
	for _, tc := range []struct{ name, script, want string }{
		{"version", `printf 'version unavailable\n'`, "single CLI version"},
		{"version-exit", `printf 'acme 1.0.0\n'; exit 1`, "exit status"},
		{"ambiguous-version", `printf 'acme v1.2.3 using runtime v4.5.6\n'`, "single CLI version"},
		{"empty", `if [ "$1" = --version ]; then echo 1.0.0; fi`, "could not recognize help"},
		{"banner", `if [ "$1" = --version ]; then echo 1.0.0; else echo 'not a help page'; fi`, "could not recognize help"},
		{"usage-error", `if [ "$1" = --version ]; then echo 1.0.0; else printf 'Usage: acme [options]\n  --help\nacme: error: unknown command\n'; exit 2; fi`, "CLI reported an error"},
		{"fatal-exit", `if [ "$1" = --version ]; then echo 1.0.0; else printf 'Usage: acme [options]\n  --help\n'; exit 3; fi`, "exit status"},
		{"output-limit", `if [ "$1" = --version ]; then echo 1.0.0; else printf 'Usage: acme [options]\n  --help\n'; head -c 1048577 /dev/zero; exit 129; fi`, "exceeds 1 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := CollectHelp(context.Background(), HelpOptions{Binary: helpExecutable(t, tc.script)})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
	binary := helpExecutable(t, "exec sleep 10")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := CollectHelp(ctx, HelpOptions{Binary: binary}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout ignored: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, _, err := CollectHelp(ctx, HelpOptions{Binary: binary}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
