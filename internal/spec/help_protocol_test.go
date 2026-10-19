package spec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const xattrHelp = `usage: xattr [-l] [-r] [-s] [-v] [-x] file [file ...]
       xattr -p [-l] [-r] [-s] [-v] [-x] attr_name file [file ...]
       xattr -w [-r] [-s] [-x] attr_name attr_value file [file ...]
       xattr -d [-r] [-s] attr_name file [file ...]
       xattr -c [-r] [-s] file [file ...]

options:
  -h: print this help
  -l: print long format
  -r: act recursively
  -s: act on the symbolic link itself
  -v: also print filename
  -x: attr_value is represented as a hex string
`

func TestSystemCLIVersions(t *testing.T) {
	for _, tc := range []struct{ name, output, want string }{
		{"tar", "bsdtar 3.5.3 - libarchive 3.7.4 zlib/1.2.12 liblzma/5.4.3 bz2lib/1.0.8", "3.5.3"},
		{"tar", "tar (GNU tar) 1.35", "1.35"},
		{"bash", "GNU bash, version 5.3.9(1)-release (aarch64-apple-darwin25.6.0)", "5.3.9"},
		{"bash", "GNU bash, version 5.2.15(1)-release (x86_64-pc-linux-gnu)", "5.2.15"},
		{"nix", "nix (Determinate Nix 3.15.1) 2.33.0", "2.33.0"},
		{"nix", "nix (Nix) 2.24.0", "2.24.0"},
		{"nix-shell", "nix-shell (Determinate Nix 3.15.1) 2.33.0", "2.33.0"},
		{"nix-env", "nix-env (Determinate Nix 3.15.1) 2.33.0", "2.33.0"},
		{"curl", "curl 8.19.0 (aarch64-apple-darwin25.3.0) libcurl/8.19.0 OpenSSL/3.6.1", "8.19.0"},
		{"pip", "pip 26.2.1 from /lib/python3.11/site-packages/pip (python 3.11)", "26.2.1"},
		{"fzf", "0.71.0 (v0.71.0)", "0.71.0"},
		{"xattr", xattrHelp, "unknown"},
	} {
		t.Run(tc.name+"-"+tc.want, func(t *testing.T) {
			t.Setenv("POLICEDOC_TEST_OUTPUT", tc.output)
			request := "--version"
			if tc.name == "xattr" {
				request = "--help"
			}
			binary := helpExecutable(t, fmt.Sprintf(`[ "$*" = %q ] || exit 11
printf '%%s\n' "$POLICEDOC_TEST_OUTPUT"
`, request))
			named := filepath.Join(filepath.Dir(binary), tc.name)
			if err := os.Rename(binary, named); err != nil {
				t.Fatal(err)
			}
			version, evidence, err := DetectVersion(t.Context(), named)
			if err != nil || version != tc.want || !strings.HasSuffix(evidence.Reference, " "+request) {
				t.Fatalf("version=%q evidence=%+v err=%v", version, evidence, err)
			}
		})
	}
}

func TestSystemHelpPages(t *testing.T) {
	for _, tc := range []struct {
		name, help string
		flags      map[string]string
	}{
		{"install", `Usage: install [OPTION]... [-T] SOURCE DEST
  or:  install [OPTION]... SOURCE... DIRECTORY
  or:  install [OPTION]... -t DIRECTORY SOURCE...
  or:  install [OPTION]... -d DIRECTORY...

Options:
  -m, --mode=MODE  Set permission mode
`, map[string]string{"-m": "required", "--mode": "required"}},
		{"xattr", xattrHelp, map[string]string{"-h": "none", "-d": "none", "-p": "none", "-w": "none", "-c": "none", "-r": "none"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command, err := parseHelpPage(tc.help, tc.name, tc.name, nil)
			if err != nil {
				t.Fatal(err)
			}
			if command.UsageName != tc.name {
				t.Fatalf("unrecognized usage: %+v", command)
			}
			for name, want := range tc.flags {
				if flag := command.Flag(name); flag == nil || flag.Value != want {
					t.Errorf("%s: got %+v, want %s", name, flag, want)
				}
			}
			if command.Flag("-h:") != nil {
				t.Fatal("description punctuation became part of a flag")
			}
		})
	}
}

func TestGoInstallReferencedFlags(t *testing.T) {
	help := "usage: go install [build flags] [packages]\n\nFor more about build flags, see 'go help build'.\n"
	command, err := parseHelpPage(help, "go", "go", []string{"install"})
	if err != nil || command.UsageName != "go" || len(command.Flags) != 0 {
		t.Fatalf("referenced flags establish the command, not the flags themselves: %+v %v", command, err)
	}
	if _, err := parseHelpPage(help, "go", "go", []string{"build"}); err == nil {
		t.Fatal("accepted help for a different command")
	}
	if _, err := parseHelpPage(strings.ReplaceAll(help, "For more about build flags, see 'go help build'.", ""), "go", "go", []string{"install"}); err == nil {
		t.Fatal("accepted a placeholder without flag details or a help reference")
	}
}

func TestCollectNixHelp(t *testing.T) {
	pages := map[string]string{
		"--help": `Name
nix - a package manager

Synopsis

nix [option...] subcommand

where subcommand is one of the following:

Main commands:
  · nix profile - manage profiles
  · nix run - run an application

Examples
  · nix example - not a command declaration

Options

  · --option name value
    Set a configuration setting.

Options for logging

  · --verbose / -v
    Enable verbose output.
`,
		"profile --help": `Name
nix profile - manage profiles

Synopsis

nix profile [option...] subcommand

where subcommand is one of the following:
  · nix profile add - add a package

Description
Manage profiles.
`,
		"profile add --help": `Name
nix profile add - add a package

Synopsis

nix profile add [option...] installables...

Description
  │ nix profile install is an alias for nix profile add.
  │ nix other bogus is an alias for nix other add.

Options
  · --profile path
    The profile to operate on.
`,
		"run --help": `Name
nix run - run an application

Synopsis

nix run [option...] installable args...

Options

## Common evaluation options

  · --include / -I path
    Add a search path.

## Options that change environment variables

  · --set-env-var / -s name value
    Set an environment variable.

Examples
  · --not-a-flag
`,
	}
	var calls []string
	reader := func(_ context.Context, _ string, args []string) (string, error) {
		request := strings.Join(args, " ")
		calls = append(calls, request)
		page, ok := pages[request]
		if !ok {
			return "", fmt.Errorf("unexpected request %q", request)
		}
		return page, nil
	}
	collector, err := newHelpCollector(t.Context(), HelpOptions{Binary: "nix", Version: "2.33.0"}, nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.Collect(t.Context(), []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	if err := collector.CollectAliases(t.Context(), []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	if err := collector.Collect(t.Context(), []string{"run"}); err != nil {
		t.Fatal(err)
	}
	root := &collector.Snapshot.Root
	if !root.SubcommandFirst || len(root.Commands) != 2 || root.Child("example") != nil {
		t.Fatalf("incorrect command catalog: %+v", root)
	}
	if flag := root.Flag("--option"); flag == nil || flag.Value != "required" || flag.ValueCount != 2 {
		t.Fatalf("lost multi-value option: %+v", flag)
	}
	profile := root.Child("profile")
	if !profile.SubcommandFirst || profile.Child("install") != profile.Child("add") || profile.Child("bogus") != nil {
		t.Fatalf("incorrect alias mapping: %+v", profile)
	}
	for _, name := range []string{"--include", "-I"} {
		if flag := root.Child("run").Flag(name); flag == nil || flag.Value != "required" {
			t.Fatalf("lost value rule for %s: %+v", name, flag)
		}
	}
	if flag := root.Child("run").Flag("-s"); flag == nil || flag.ValueCount != 2 {
		t.Fatalf("lost option subsection: %+v", flag)
	}
	if root.Child("run").Flag("--not-a-flag") != nil {
		t.Fatal("example was parsed as an option")
	}
	want := []string{"--help", "profile --help", "profile add --help", "run --help"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%q want=%q", calls, want)
	}
}
