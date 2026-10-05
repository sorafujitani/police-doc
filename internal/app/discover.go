package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sorafujitani/police-doc/internal/check"
	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

type collectedHelp struct {
	cachedHelp
	fresh bool
	err   error
}

type collectionOptions struct {
	CacheDir string
	Refresh  bool
}

func collectAndCheck(ctx context.Context, example extract.Example, cache map[string]collectedHelp, options collectionOptions) (check.Result, error) {
	if example.Reason != "" || example.CLI() == "" {
		return check.Example(example, nil), ctx.Err()
	}
	name := example.CLI()
	collected := discoverHelp(ctx, example, cache, options)
	if err := ctx.Err(); err != nil {
		return check.Result{}, err
	}
	result := check.Example(example, collected.Snapshot)
	if collected.err != nil {
		result.Diagnostics = append(result.Diagnostics, check.Diagnostic{
			Severity: "warning", Status: "uncheckable", Code: "help-unavailable",
			Message:  fmt.Sprintf("Could not collect help for %q.", name),
			Detail:   fmt.Sprintf("%.500s; no package was installed and no wrapper was executed.", collected.err),
			Evidence: []spec.Evidence{},
		})
	} else if len(collected.Warnings) > 0 {
		result.Diagnostics = append(result.Diagnostics, check.Diagnostic{
			Severity: "warning", Status: "needs-review", Code: "help-incomplete",
			Message: strings.Join(collected.Warnings, "\n"), Evidence: collected.Snapshot.Sources,
		})
	}
	return result, nil
}

func discoverHelp(ctx context.Context, example extract.Example, memory map[string]collectedHelp, options collectionOptions) (collected collectedHelp) {
	name := example.CLI()
	path, err := helpCachePath(options.CacheDir, example)
	if err != nil {
		return collectedHelp{err: err}
	}
	requestKey := "request\x00" + path
	if previous, ok := memory[requestKey]; ok {
		return previous
	}
	defer func() { memory[requestKey] = collected }()
	saved, cacheErr := loadHelpCache(path, name)
	binary, err := helpExecutable(example)
	if err != nil {
		return collectedHelp{err: err}
	}
	fingerprint, err := executableFingerprint(binary)
	if err != nil {
		return collectedHelp{err: err}
	}
	binaryKey := "binary\x00" + name + "\x00" + binary + "\x00" + fingerprint
	collected, exists := memory[binaryKey]
	if !exists {
		version, evidence, err := spec.DetectVersion(ctx, binary)
		if err != nil {
			collected.err = err
		} else if !options.Refresh && cacheErr == nil && saved.Binary == binary && saved.Fingerprint == fingerprint && saved.Snapshot.Version == version {
			collected.cachedHelp = saved
		} else {
			collected.cachedHelp = cachedHelp{SchemaVersion: helpCacheVersion, Binary: binary, Fingerprint: fingerprint}
			collected.Snapshot, collected.Warnings, collected.err = spec.CollectHelp(ctx, spec.HelpOptions{
				Binary: binary, Tool: name, Version: version,
			})
			if collected.err == nil {
				collected.Snapshot.Sources = append([]spec.Evidence{evidence}, collected.Snapshot.Sources...)
				collected.fresh = true
			}
		}
		memory[binaryKey] = collected
	}
	if ctx.Err() == nil && collected.err == nil && (collected.fresh || cacheErr != nil || saved.Binary != binary || saved.Fingerprint != fingerprint || saved.Snapshot.Version != collected.Snapshot.Version) {
		if err := writeHelpCache(path, collected.cachedHelp); err != nil {
			// Do not store a cache-write failure as part of the collected CLI specification.
			collected.Warnings = append(slices.Clone(collected.Warnings), fmt.Sprintf("Could not save help cache: %v", err))
		}
	}
	return collected
}

func helpExecutable(example extract.Example) (string, error) {
	name := example.Words[0].Value
	if example.LocalPackage {
		dir, err := filepath.Abs(filepath.Dir(example.Location.File))
		if err != nil {
			return "", err
		}
		for {
			candidate := filepath.Join(dir, "node_modules", ".bin", name)
			if binary, err := exec.LookPath(candidate); err == nil {
				return binary, nil
			} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, exec.ErrNotFound) {
				return "", err // Do not replace a broken local installation with a different global version.
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	binary, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(binary)
}
