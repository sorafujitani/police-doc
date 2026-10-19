package app

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/sorafujitani/police-doc/internal/check"
	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

func collectBuiltin(ctx context.Context, example extract.Example, cache map[string]*collectedHelp, options collectionOptions) (check.Result, error) {
	key := spec.TargetKey(example.CLI(), "bash")
	collected := cache[key]
	if collected == nil {
		collected = &collectedHelp{}
		binary, err := exec.LookPath("bash")
		collected.err = err
		if err == nil {
			collected.Snapshot, collected.err = spec.CollectBuiltinHelp(ctx, binary, example.CLI())
		}
		cache[key] = collected
	}
	result := check.Example(example, collected.Snapshot)
	result.SpecSource = "shell-help"
	if collected.err != nil {
		result.Diagnostics = append(result.Diagnostics, check.Diagnostic{Severity: "warning", Status: "uncheckable", Code: "builtin-help-unavailable", Message: "Could not collect the shell's own builtin help.", Detail: fmt.Sprintf("%.500s", collected.err), Evidence: []spec.Evidence{}})
	}
	return checkForwarded(ctx, result, cache, options)
}
