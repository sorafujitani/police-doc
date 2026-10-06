package check

import (
	"strings"

	"github.com/sorafujitani/police-doc/internal/spec"
)

// Exact names win over clusters, preserving single-dash long flags. A cluster
// is expanded only when every component and its value boundary are documented.
func flagWords(command *spec.Command, word string) []string {
	name, _, _ := strings.Cut(word, "=")
	if command.Flag(name) != nil || !command.ShortFlagClusters || !strings.HasPrefix(word, "-") || strings.HasPrefix(word, "--") || len(word) < 3 {
		return []string{word}
	}
	var words []string
	for i := 1; i < len(word); i++ {
		name := "-" + word[i:i+1]
		flag := command.Flag(name)
		if flag == nil || flag.Value == "unknown" {
			return []string{word}
		}
		if flag.Value != "none" && i+1 < len(word) {
			return append(words, name+"="+strings.TrimPrefix(word[i+1:], "="))
		}
		words = append(words, name)
	}
	return words
}
