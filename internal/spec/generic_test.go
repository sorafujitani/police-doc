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
`, []string{"clone", "init"}, map[string]string{"-v": "none", "--version": "none", "-C": "required", "--exec-path": "optional-attached"}},
		{"git-flags", `usage: git commit [options]
    -m, --[no-]message <message>  commit message
    -S, --[no-]gpg-sign[=<key-id>]
    --trailer <trailer-value>  add a trailer (see --not-a-flag)
`, nil, map[string]string{"-m": "required", "--message": "required", "--no-message": "unknown", "-S": "optional-attached", "--gpg-sign": "optional-attached", "--no-gpg-sign": "unknown", "--trailer": "required"}},
		{"npm-root", `npm <command>

All commands:
    install, run, test,
    version

Options:
    --help  Print help
`, []string{"install", "run", "test", "version"}, map[string]string{"--help": "none"}},
		{"npm-flags", `Usage:
npm install [<package-spec> ...]

Options:
[-E|--save-exact] [-g|--global]
[--omit <dev|optional|peer> [--omit <dev|optional|peer> ...]]
[-w|--workspace <workspace-name> [-w|--workspace <workspace-name> ...]]
  --omit
    Dependency types to omit.
`, nil, map[string]string{"-E": "none", "--save-exact": "none", "-g": "none", "--global": "none", "--omit": "required", "-w": "required", "--workspace": "required"}},
		{"brew-subcommands", `Usage: brew services [subcommand]

Subcommands:
  list:
    List services.
  start:
    Start a service.

Options:
  --json  JSON output
`, []string{"list", "start"}, map[string]string{"--json": "none"}},
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
`, []string{"run", "container", "compose"}, map[string]string{"--config": "required", "-H": "required", "--host": "required", "--debug": "none"}},
		{"other-formats", "Usage: acme [OPTIONS]\n\nOptions:\n\t-o, --output FILE\tOutput file\n  --color[=WHEN]  Color mode\n  --dry_run  Dry run\n  \x1b[32m--define\x1b[0m=<key=value>  Define value\n  --mode <--fast|--slow>  Modes, not flags\n", nil, map[string]string{"-o": "required", "--output": "required", "--color": "optional-attached", "--dry_run": "none", "--define": "required", "--mode": "required"}},
		{"uv-repeated-flags", "Options:\n  -q, --quiet...\n  -v, --verbose...\n", nil, map[string]string{"-q": "none", "--quiet": "none", "-v": "none", "--verbose": "none"}},
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
			plain, err := normalizeTerminalOutput(tc.help)
			if err != nil {
				t.Fatal(err)
			}
			command := parseHelp(plain)
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

func collectTestPaths(t *testing.T, options HelpOptions, paths ...string) *Snapshot {
	t.Helper()
	collector, err := NewHelpCollector(t.Context(), options, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if err := collector.Collect(t.Context(), strings.Fields(path)); err != nil {
			t.Fatal(err)
		}
	}
	return collector.Snapshot
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
			contract := collectTestPaths(t, HelpOptions{Binary: binary})
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
	snapshot := collectTestPaths(t, HelpOptions{Binary: binary}, "container", "container run")
	calls, err := os.ReadFile(log)
	if err != nil || string(calls) != "--version\n-h\ncontainer -h\ncontainer run -h\n" {
		t.Fatalf("unexpected calls: %q %v", calls, err)
	}
	run := snapshot.Root.Child("container").Child("run")
	if snapshot.Tool != "acme" || snapshot.Version != "1.2.3-beta.1" || snapshot.OS != runtime.GOOS || len(snapshot.Sources) != 4 || len(run.Flags) != 2 || run.Flags[0].Value != "required" || len(run.Sources) != 1 {
		t.Fatalf("unexpected snapshot: %+v; run=%+v", snapshot, run)
	}
}

func TestCollectHelpUsageOnly(t *testing.T) {
	binary := helpExecutable(t, `
case "$*" in
  --version) echo 1.0.0 ;;
  -h) printf 'Usage:\n  acme completion  List completions\n  acme shrinkwrap  Create a lockfile\n\nAll commands:\n  completion, shrinkwrap\n' ;;
  'completion -h') printf 'Tab Completion\n\nUsage:\nacme completion\n\nOptions:\n' ;;
  'shrinkwrap -h') printf 'Lock dependencies\n\nUsage:\nacme shrinkwrap\n'; exit 1 ;;
  *) exit 11 ;;
esac
`)
	snapshot := collectTestPaths(t, HelpOptions{Binary: binary}, "completion", "shrinkwrap")
	for _, name := range []string{"completion", "shrinkwrap"} {
		child := snapshot.Root.Child(name)
		if child == nil || len(child.Sources) != 1 || len(child.Flags) != 0 || len(child.Commands) != 0 {
			t.Fatalf("usage-only command must have evidence, not a placeholder: %+v", child)
		}
	}
	root := helpExecutable(t, `printf '\033[32mUsage:\033[0m acme FILE\n'`)
	if snapshot := collectTestPaths(t, HelpOptions{Binary: root, Version: "1.0.0"}); len(snapshot.Root.Sources) != 1 {
		t.Fatalf("usage-only root was rejected: %+v", snapshot)
	}
}

func TestCollectBrewHelp(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("POLICEDOC_TEST_CALLS", log)
	binary := helpExecutable(t, `
printf '%s\n' "$*" >> "$POLICEDOC_TEST_CALLS"
case "$*" in
  --version) echo 'Homebrew 5.0.0' ;;
  'commands --quiet') printf '%s\n' --prefix tap install services help tap 'bad;touch marker' ../escape ;;
  'help tap') printf 'Usage: brew tap [options] [user/repo]\n  --force  Force tap\n' ;;
  'help install') printf 'Usage: brew install [options] formula|cask [...]\n  --formula  Install a formula\n' ;;
  'help services') printf 'Usage: brew services [subcommand]\nSubcommands:\n  list:\n    List services\n' ;;
  'help services list') printf 'Usage: brew services list\n  --json  JSON output\n' ;;
  'help help') printf 'Example usage:\n  brew install FORMULA\n\nFurther help:\n  brew commands\n  brew help [COMMAND]\n' ;;
  *) echo 'error: unexpected execution'; exit 11 ;;
esac
`)
	brew := filepath.Join(filepath.Dir(binary), "brew")
	if err := os.Rename(binary, brew); err != nil {
		t.Fatal(err)
	}
	zero := 0
	for _, tc := range []struct {
		name  string
		depth *int
		limit int
		calls string
		warn  string
	}{
		{"full", nil, 6, "--version\ncommands --quiet\nhelp tap\nhelp install\nhelp services\nhelp help\nhelp services list\n", ""},
		{"call-limit", nil, 2, "--version\ncommands --quiet\nhelp tap\n", "help call limit"},
		{"depth-limit", &zero, 100, "--version\ncommands --quiet\n", "help depth limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			collector, err := NewHelpCollector(t.Context(), HelpOptions{Binary: brew, MaxDepth: tc.depth, MaxCommands: tc.limit}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"tap", "install", "services", "help", "services list"} {
				err = collector.Collect(t.Context(), strings.Fields(path))
				if err != nil {
					break
				}
			}
			if (tc.warn == "" && err != nil) || (tc.warn != "" && (err == nil || !strings.Contains(err.Error(), tc.warn))) {
				t.Fatalf("expected %q, got %v", tc.warn, err)
			}
			snapshot := collector.Snapshot
			calls, err := os.ReadFile(log)
			if err != nil || string(calls) != tc.calls {
				t.Fatalf("unexpected brew calls: %q %v", calls, err)
			}
			if len(snapshot.Root.Commands) != 4 || len(snapshot.Root.Sources) != 1 || len(snapshot.Root.Flags) != 1 || snapshot.Root.Flags[0].Name != "--prefix" {
				t.Fatalf("invalid brew command list: %+v", snapshot.Root)
			}
			if tc.warn != "" {
				return
			}
			for _, name := range []string{"tap", "install"} {
				child := snapshot.Root.Child(name)
				if child == nil || len(child.Flags) != 1 || len(child.Sources) != 1 {
					t.Fatalf("brew %s was not collected: %+v", name, child)
				}
			}
			list := snapshot.Root.Child("services").Child("list")
			if list == nil || len(list.Flags) != 1 || list.Flags[0].Name != "--json" || len(list.Sources) != 1 {
				t.Fatalf("brew services list was not collected: %+v", list)
			}
		})
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
  "$POLICEDOC_TEST_CHILD -h") printf 'Usage: quartz %s COMMAND\nSubcommands:\n  leaf  Nested command\n' "$POLICEDOC_TEST_CHILD" ;;
  "$POLICEDOC_TEST_CHILD leaf -h") printf 'Usage: quartz %s leaf [OPTIONS]\nOptions:\n  --destination <file>  Output file\n' "$POLICEDOC_TEST_CHILD" ;;
  *) exit 11 ;;
esac
`)
	for _, name := range []string{"fresh-command", "newly-added"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("POLICEDOC_TEST_CHILD", name)
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			snapshot := collectTestPaths(t, HelpOptions{Binary: binary, Tool: "quartz"}, name, name+" leaf")
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
  unavailable*) exit 3 ;;
  empty*) echo 'No supported help format' ;;
  recursive*) printf 'Usage: acme recursive COMMAND\nCommands:\n  recursive  Repeating help\n' ;;
  *) exit 11 ;;
esac
`)
	zero, one := 0, 1
	for _, tc := range []struct {
		name         string
		depth        *int
		limit, calls int
		paths        []string
		want         string
	}{
		{"unused", nil, 100, 1, nil, ""},
		{"root-only", &zero, 100, 1, []string{"branch"}, "help depth limit"},
		{"one-level", &one, 100, 2, []string{"branch", "branch leaf"}, "help depth limit"},
		{"call-limit", nil, 2, 2, []string{"branch", "branch leaf"}, "help call limit"},
		{"failure-reuse", nil, 100, 3, []string{"unavailable", "unavailable"}, "exit status"},
		{"unparsed", nil, 100, 3, []string{"empty"}, "could not recognize help"},
		{"parent-help", nil, 100, 4, []string{"recursive", "recursive recursive"}, "Usage does not describe"},
		{"unadvertised", nil, 100, 1, []string{"unknown"}, "not advertised"},
		{"uncollected-parent", nil, 100, 1, []string{"branch leaf"}, "not advertised"},
		{"unsafe-name", nil, 100, 1, []string{"../escape"}, "not advertised"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			collector, err := NewHelpCollector(t.Context(), HelpOptions{Binary: binary, Version: "1.0.0", MaxDepth: tc.depth, MaxCommands: tc.limit}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range tc.paths {
				err = collector.Collect(t.Context(), strings.Fields(path))
			}
			if (tc.want == "" && err != nil) || (tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want))) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
			calls, err := os.ReadFile(log)
			if err != nil || strings.Count(string(calls), "\n") != tc.calls {
				t.Fatalf("unexpected calls: %q %v", calls, err)
			}
			if len(collector.Snapshot.Root.Child("unavailable").Sources) != 0 {
				t.Fatal("failed or unused command must remain a placeholder")
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
	collector, err := NewHelpCollector(t.Context(), HelpOptions{Binary: binary, Version: "1.0.0"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := collector.Collect(ctx, []string{"slow"}); !errors.Is(err, context.DeadlineExceeded) {
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
		if _, err := NewHelpCollector(t.Context(), options, nil); err == nil {
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
		collector, err := NewHelpCollector(t.Context(), HelpOptions{Binary: binary, MaxCommands: limit}, nil)
		if err == nil {
			err = collector.Collect(t.Context(), []string{"next"})
		}
		calls, readErr := os.ReadFile(log)
		if readErr != nil || strings.Count(string(calls), "\n") != limit+1 {
			t.Fatalf("call budget %d violated: %q %v", limit, calls, readErr)
		}
		if limit == 1 {
			if err == nil || collector != nil {
				t.Fatal("root fallback exceeded its budget")
			}
			continue
		}
		child := collector.Snapshot.Root.Child("next")
		if limit == 3 && (err == nil || len(child.Sources) != 0) {
			t.Fatal("limited fallback did not leave a warned placeholder")
		}
		if limit == 4 && (err != nil || len(child.Flags) != 1 || !strings.HasSuffix(child.Sources[0].Reference, "next --help")) {
			t.Fatalf("fallback failed: %+v %v", child, err)
		}
	}
}

func TestHelpDisplayNameSurvivesCaching(t *testing.T) {
	binary := helpExecutable(t, `
case "$*" in
  --version) echo 1.0.0 ;;
  -h) printf 'Usage: /runtime/Canonical COMMAND\nCommands:\n  run  Run something\n  next  Next command\n' ;;
  'run -h') printf 'Usage: /runtime/Canonical run\n' ;;
  'next -h') printf 'Usage: /runtime/Canonical next\n' ;;
  *) exit 11 ;;
esac
`)
	snapshot := collectTestPaths(t, HelpOptions{Binary: binary}, "run")
	if snapshot.Root.UsageName != "Canonical" || snapshot.Tool != "acme" {
		t.Fatalf("invocation and help names were conflated: %+v", snapshot)
	}
	collector, err := NewHelpCollector(t.Context(), HelpOptions{Binary: binary, Version: snapshot.Version}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.Collect(t.Context(), []string{"next"}); err != nil {
		t.Fatal(err)
	}
	if collector.Revision != 1 {
		t.Fatal("cached root help was recollected")
	}
}

func TestCollectHelpFailures(t *testing.T) {
	for _, tc := range []struct{ name, script, want string }{
		{"version", `printf 'version unavailable\n'`, "single CLI version"},
		{"version-exit", `printf 'acme 1.0.0\n'; exit 1`, "exit status"},
		{"ambiguous-version", `printf 'acme v1.2.3 using runtime v4.5.6\n'`, "single CLI version"},
		{"empty", `if [ "$1" = --version ]; then echo 1.0.0; fi`, "could not recognize help"},
		{"banner", `if [ "$1" = --version ]; then echo 1.0.0; else echo 'not a help page'; fi`, "could not recognize help"},
		{"empty-usage", `if [ "$1" = --version ]; then echo 1.0.0; else printf 'Usage:\n\nOptions:\n'; fi`, "Usage does not describe"},
		{"invalid-usage", `if [ "$1" = --version ]; then echo 1.0.0; else echo 'Usage: <program>'; fi`, "Usage does not describe"},
		{"unparsed-commands", `if [ "$1" = --version ]; then echo 1.0.0; else echo 'Usage: acme COMMAND'; fi`, "could not recognize help"},
		{"unparsed-options", `if [ "$1" = --version ]; then echo 1.0.0; else echo 'Usage: acme [OPTIONS]'; fi`, "could not recognize help"},
		{"usage-error", `if [ "$1" = --version ]; then echo 1.0.0; else printf 'Usage: acme [options]\n  --help\nacme: error: unknown command\n'; exit 2; fi`, "CLI reported an error"},
		{"fatal-exit", `if [ "$1" = --version ]; then echo 1.0.0; else printf 'Usage: acme [options]\n  --help\n'; exit 3; fi`, "exit status"},
		{"output-limit", `if [ "$1" = --version ]; then echo 1.0.0; else printf 'Usage: acme [options]\n  --help\n'; head -c 1048577 /dev/zero; exit 129; fi`, "exceeds 1 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHelpCollector(t.Context(), HelpOptions{Binary: helpExecutable(t, tc.script)}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
	binary := helpExecutable(t, "exec sleep 10")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := NewHelpCollector(ctx, HelpOptions{Binary: binary}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout ignored: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := NewHelpCollector(ctx, HelpOptions{Binary: binary}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
