package app

import (
	"fmt"
	"maps"
	"slices"

	"github.com/sorafujitani/police-doc/internal/spec"
)

// Export preserves the rules actually collected. In particular, exporting help
// does not invent positional bounds or turn an open flag list into a closed one.
func exportReferences(path string, references map[string]*spec.Snapshot, collected map[string]*collectedHelp) error {
	targets := maps.Clone(references)
	if targets == nil {
		targets = make(map[string]*spec.Snapshot)
	}
	for _, help := range collected {
		if help.err != nil || help.Snapshot == nil {
			continue
		}
		target := help.Snapshot
		if previous := targets[target.Key()]; previous != nil && previous != target {
			return fmt.Errorf("cannot export multiple targets for %s", target.Tool)
		}
		targets[target.Key()] = target
	}
	if len(targets) == 0 {
		return fmt.Errorf("no specifications are available to export")
	}
	document := spec.References{SchemaVersion: 1}
	for _, name := range slices.Sorted(maps.Keys(targets)) {
		document.Tools = append(document.Tools, *targets[name])
	}
	return writeJSONFile(path, document)
}
