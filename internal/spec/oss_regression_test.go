package spec

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestUsageStopsBeforeUnindentedDescription(t *testing.T) {
	for _, name := range []string{"mkdir", "ls", "cat", "tee", "tail"} {
		help := "Usage: " + name + " [OPTION]... FILE...\nRead or write the specified files.\nMore description, without a blank line.\n\nOptions:\n  --output FILE  Output file\n"
		command, err := parseHelpPage(help, name, name, nil)
		if err != nil || command.UsageName != name || command.Flag("--output") == nil {
			t.Errorf("%s: %+v %v", name, command, err)
		}
	}
}

func TestCurlHelpUsesFullCatalog(t *testing.T) {
	reader := func(_ context.Context, _ string, args []string) (string, error) {
		if !slices.Equal(args, []string{"--help", "all"}) {
			t.Fatalf("expected curl's full option catalog, got %q", args)
		}
		return "Usage: curl [options...] <url>\n -L, --location  Follow redirects\n -S, --show-error  Show errors\n -o, --output <file>  Output file\n", nil
	}
	collector, err := newHelpCollector(t.Context(), HelpOptions{Binary: "curl", Version: "8.19.0", MaxCommands: 1}, nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	if root := &collector.Snapshot.Root; !root.ShortFlagClusters || root.Flag("-L") == nil || root.Flag("-o").Value != "required" {
		t.Fatalf("lost curl flags: %+v", root)
	}
}

func TestFzfLongSignaturesKeepProseOutOfArity(t *testing.T) {
	help := "Usage: fzf [options]\nOptions:\n    --height=[~][-]HEIGHT[%] Display window below the cursor\n    --min-height=HEIGHT[+]   Minimum height\n"
	command, err := parseHelpPage(help, "fzf", "fzf", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"--height", "--min-height"} {
		if flag := command.Flag(name); flag == nil || flag.Value != "required" || flag.ValueCount != 0 {
			t.Errorf("prose changed the value boundary: %+v", flag)
		}
	}
}

func TestRipgrepOtherBehaviors(t *testing.T) {
	command, err := parseHelpPage("Usage: rg [OPTIONS] PATTERN\n\nOTHER BEHAVIORS:\n  --type-list  Show all supported file types.\n  --files  Print each file that would be searched.\n", "rg", "rg", nil)
	if err != nil || command.Flag("--type-list") == nil || command.Flag("--files") == nil {
		t.Fatalf("lost ripgrep option group: %+v %v", command, err)
	}
}

func TestRustupExecutableSuffix(t *testing.T) {
	help := "Usage: rustup[EXE] target <COMMAND>\n\nCommands:\n  add  Add a target\n"
	command, err := parseHelpPage(help, "rustup", "rustup", []string{"target"})
	if err != nil || command.UsageName != "rustup" || command.Child("add") == nil {
		t.Fatalf("optional executable suffix was not recognized: %+v %v", command, err)
	}
	if _, err := parseHelpPage(help, "rustup", "rustup", []string{"toolchain"}); err == nil {
		t.Fatal("suffix normalization accepted a different command's help")
	}
}

func TestAdvertisedLongHelp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		fail  bool
	}{
		{"full", 2, false},
		{"budget", 1, false},
		{"fallback", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			reader := func(_ context.Context, _ string, args []string) (string, error) {
				request := strings.Join(args, " ")
				calls = append(calls, request)
				switch request {
				case "-h":
					return "Usage: acme [OPTIONS] [FILE]\nOptions:\n  -h, --help  Print help (see more with '--help')\n", nil
				case "--help":
					if tc.fail {
						return "", errors.New("long help unavailable")
					}
					return "Usage: acme [OPTIONS] [FILE]\nOptions:\n  --config-file  Print config path\n  -h, --help  Print help\n", nil
				default:
					t.Fatalf("unexpected metadata request: %q", request)
					return "", nil
				}
			}
			collector, err := newHelpCollector(t.Context(), HelpOptions{Binary: "acme", Version: "1.0.0", MaxCommands: tc.limit}, nil, reader)
			if err != nil {
				t.Fatal(err)
			}
			root := &collector.Snapshot.Root
			full := tc.limit == 2 && !tc.fail
			if (root.Flag("--config-file") != nil) != full {
				t.Fatalf("wrong help selected: %+v", root)
			}
			want := []string{"-h", "--help"}[:tc.limit]
			if !slices.Equal(calls, want) {
				t.Fatalf("calls=%q want=%q", calls, want)
			}
			reference := "acme -h"
			if full {
				reference = "acme --help"
			}
			if len(root.Sources) != 1 || root.Sources[0].Reference != reference {
				t.Fatalf("incorrect evidence: %+v", root.Sources)
			}
		})
	}
}
