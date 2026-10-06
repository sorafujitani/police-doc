package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/sorafujitani/police-doc/internal/check"
	"github.com/sorafujitani/police-doc/internal/extract"
)

const usage = `policedoc — check Markdown CLI examples against installed CLI help

Usage:
  policedoc scan <file.md|directory>...
                [--refresh] [--cache-dir directory]
                [--format text|json] [--verbose] [--fail-on error|warning|none]
  policedoc version

scan never executes document commands. It starts trusted executables for version
and help requests, caching the results for new or changed CLIs.
sudo/npx are never executed; packages are never installed.
Help is incomplete: unknown flags need review, not automatic rejection.
Missing values explicitly documented in help are errors.
Text shows errors and warnings; --verbose adds INFO messages and collection details.
INFO means incomplete checking, not validation success. JSON always includes all details.
Default failure threshold: error. Exit codes: 0 = below threshold;
1 = findings at threshold; 2 = operational error.
`

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if _, err := io.WriteString(stdout, usage); err != nil {
			fmt.Fprintln(stderr, "policedoc:", err)
			return 2
		}
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, "policedoc: version takes no arguments")
			return 2
		}
		if _, err := fmt.Fprintf(stdout, "policedoc %s (%s)\n", version, runtime.Version()); err != nil {
			fmt.Fprintln(stderr, "policedoc:", err)
			return 2
		}
		return 0
	}
	var code int
	var err error
	switch args[0] {
	case "scan":
		code, err = scan(ctx, args[1:], stdout, stderr)
	default:
		err = fmt.Errorf("unknown command %q; use policedoc --help", args[0])
	}
	if err != nil {
		fmt.Fprintln(stderr, "policedoc:", err)
		return 2
	}
	return code
}

func scan(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "text or json")
	verbose := flags.Bool("verbose", false, "include info messages, target versions, and collection details in text output")
	failOn := flags.String("fail-on", "error", "error, warning, or none")
	refresh := flags.Bool("refresh", false, "refresh help for all CLIs used in the documents")
	cacheDir := flags.String("cache-dir", ".policedoc/cache", "directory for automatically collected CLI specifications")
	var usageErr error
	flags.Usage = func() { _, usageErr = io.WriteString(stdout, usage) }
	args, err := interspersed(flags, args)
	if err != nil {
		return 0, err
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, usageErr
		}
		return 0, err
	}
	collection := collectionOptions{CacheDir: *cacheDir, Refresh: *refresh}
	if *cacheDir == "" {
		return 0, fmt.Errorf("cache-dir must not be empty")
	}
	if *format != "text" && *format != "json" {
		return 0, fmt.Errorf("format must be text or json")
	}
	if flags.NArg() == 0 {
		return 0, fmt.Errorf("scan requires at least one Markdown file or directory")
	}
	if *failOn != "error" && *failOn != "warning" && *failOn != "none" {
		return 0, fmt.Errorf("fail-on must be error, warning, or none")
	}
	files, err := markdownFiles(flags.Args())
	if err != nil {
		return 0, err
	}
	report := check.NewReport(len(files))
	collected := make(map[string]*collectedHelp)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		file, err := os.Open(path)
		if err != nil {
			return 0, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, (16<<20)+1))
		closeErr := file.Close()
		if readErr != nil {
			return 0, fmt.Errorf("%s: %w", path, readErr)
		}
		if closeErr != nil {
			return 0, closeErr
		}
		if len(data) > 16<<20 {
			return 0, fmt.Errorf("%s: Markdown exceeds 16 MiB", path)
		}
		for _, example := range extract.Markdown(path, data) {
			result, err := collectAndCheck(ctx, example, collected, collection)
			if err != nil {
				return 0, err
			}
			report.Add(result)
		}
	}
	if err := report.Write(stdout, *format, *verbose); err != nil {
		return 0, err
	}
	if report.Fails(*failOn) {
		return 1, nil
	}
	return 0, nil
}

// Standard flag parsing stops at the first positional argument; scan deliberately
// accepts options after input paths too, as shown in the public CLI examples.
func interspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	var options, paths []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			paths = append(paths, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			paths = append(paths, arg)
			continue
		}
		name, _, attached := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name == "help" || name == "h" {
			return []string{"-h"}, nil
		}
		option := flags.Lookup(name)
		if option == nil {
			return nil, fmt.Errorf("unknown option %q", arg)
		}
		options = append(options, arg)
		if !attached {
			boolean, ok := option.Value.(interface{ IsBoolFlag() bool })
			if !ok || !boolean.IsBoolFlag() {
				if i+1 == len(args) {
					return nil, fmt.Errorf("%s requires a value", arg)
				}
				i++
				options = append(options, args[i])
			}
		}
	}
	return append(append(options, "--"), paths...), nil
}

func markdownFiles(inputs []string) ([]string, error) {
	found := make(map[string]string)
	add := func(path string) error {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if _, exists := found[absolute]; !exists {
			found[absolute] = filepath.Clean(path)
		}
		return nil
	}
	for _, input := range inputs {
		info, err := os.Lstat(input)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: symbolic-link inputs are not followed", input)
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() || !isMarkdown(input) {
				return nil, fmt.Errorf("%s: expected a Markdown file or directory", input)
			}
			if err := add(input); err != nil {
				return nil, err
			}
			continue
		}
		err = filepath.WalkDir(input, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type().IsRegular() && isMarkdown(path) {
				return add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return slices.Sorted(maps.Values(found)), nil
}

func isMarkdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}
