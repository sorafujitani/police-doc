package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sorafujitani/police-doc/internal/check"
	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

type collectedHelp struct {
	cachedHelp
	collector     *spec.HelpCollector
	err           error
	cachePath     string
	savedRevision int
	cacheErr      error
}

type collectionOptions struct {
	CacheDir string
	Refresh  bool
}

func collectAndCheck(ctx context.Context, example extract.Example, cache map[string]*collectedHelp, options collectionOptions) (check.Result, error) {
	if example.Reason != "" || example.CLI() == "" {
		return check.Example(example, nil), ctx.Err()
	}
	collected := discoverHelp(ctx, example, cache, options)
	result := check.Example(example, collected.Snapshot)
	for collected.err == nil && len(result.HelpPath) > 0 {
		if err := collected.collector.Collect(ctx, result.HelpPath); err != nil {
			result.Diagnostics = append(result.Diagnostics, check.Diagnostic{
				Severity: "warning", Status: "uncheckable", Code: "help-incomplete",
				Message: fmt.Sprintf("Could not collect help for %q.", strings.Join(append([]string{example.CLI()}, result.HelpPath...), " ")),
				Detail:  fmt.Sprintf("%.500s", err), Evidence: []spec.Evidence{},
			})
			break
		}
		result = check.Example(example, collected.Snapshot)
	}
	if err := ctx.Err(); err != nil {
		return check.Result{}, err
	}
	if collected.err != nil {
		result.Diagnostics = append(result.Diagnostics, check.Diagnostic{
			Severity: "warning", Status: "uncheckable", Code: "help-unavailable",
			Message:  fmt.Sprintf("Could not collect help for %q.", example.CLI()),
			Detail:   fmt.Sprintf("%.500s; no package was installed and no wrapper was executed.", collected.err),
			Evidence: []spec.Evidence{},
		})
	} else {
		if collected.savedRevision != collected.collector.Revision {
			collected.cacheErr = writeHelpCache(collected.cachePath, collected.cachedHelp)
			collected.savedRevision = collected.collector.Revision
		}
		if collected.cacheErr != nil {
			result.Diagnostics = append(result.Diagnostics, check.Diagnostic{
				Severity: "warning", Status: "needs-review", Code: "help-cache-unavailable",
				Message: fmt.Sprintf("Could not save help cache: %v", collected.cacheErr), Evidence: []spec.Evidence{},
			})
		}
	}
	return result, nil
}

func discoverHelp(ctx context.Context, example extract.Example, memory map[string]*collectedHelp, options collectionOptions) (collected *collectedHelp) {
	name := example.CLI()
	binary, err := helpExecutable(example)
	if err != nil {
		return &collectedHelp{err: err}
	}
	fingerprint, err := executableFingerprint(binary)
	if err != nil {
		return &collectedHelp{err: err}
	}
	path, err := helpCachePath(options.CacheDir, binary, name)
	if err != nil {
		return &collectedHelp{err: err}
	}
	binaryKey := path + "\x00" + fingerprint
	if previous, ok := memory[binaryKey]; ok {
		return previous
	}
	collected = &collectedHelp{cachePath: path, savedRevision: -1}
	defer func() { memory[binaryKey] = collected }()
	version, evidence, err := spec.DetectVersion(ctx, binary)
	if err != nil {
		collected.err = err
		return collected
	}
	saved, cacheErr := loadHelpCache(path, name)
	fromCache := !options.Refresh && cacheErr == nil && saved.Binary == binary && saved.Fingerprint == fingerprint && saved.Snapshot.Version == version
	if fromCache {
		collected.cachedHelp, collected.savedRevision = saved, 0
	} else {
		collected.cachedHelp = cachedHelp{SchemaVersion: helpCacheVersion, Binary: binary, Fingerprint: fingerprint}
	}
	collected.collector, collected.err = spec.NewHelpCollector(ctx, spec.HelpOptions{
		Binary: binary, Tool: name, Version: version,
	}, collected.Snapshot)
	if collected.err == nil {
		collected.Snapshot = collected.collector.Snapshot
		if !fromCache {
			collected.Snapshot.Sources = append([]spec.Evidence{evidence}, collected.Snapshot.Sources...)
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
