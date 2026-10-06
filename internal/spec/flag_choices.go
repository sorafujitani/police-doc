package spec

import (
	"regexp"
	"strings"
)

var (
	helpChoiceSet       = regexp.MustCompile(`([{<])([A-Za-z0-9_.:/+-]+(?:[ \t]*[|,][ \t]*[A-Za-z0-9_.:/+-]+)+)([}>])`)
	helpExplicitChoices = regexp.MustCompile(`\b(?:choices|(?:allowed|possible|accepted) values|one of)\b`)
)

// Accept explicit enumerations in a value signature or after a description's
// colon, not arbitrary words/examples in prose. Mismatches remain review findings.
func helpFlagChoices(signature, description string) []string {
	match := helpChoiceSet.FindStringSubmatch(signature)
	if match == nil {
		if colon := strings.Index(description, ":"); colon >= 0 {
			label := strings.ToLower(strings.TrimSpace(description[:colon]))
			if strings.Contains(label, "example") || (strings.Contains(label, "default") && !helpExplicitChoices.MatchString(label)) {
				return nil // Examples and defaults do not restrict allowed values.
			}
			text := strings.TrimSpace(description[colon+1:])
			if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "<") {
				match = helpChoiceSet.FindStringSubmatch(text)
			}
		}
	}
	if match == nil || (match[1] == "{" && match[3] != "}") || (match[1] == "<" && match[3] != ">") {
		return nil
	}
	var choices []string
	seen := make(map[string]bool)
	for _, choice := range strings.FieldsFunc(match[2], func(r rune) bool { return r == '|' || r == ',' }) {
		choice = strings.TrimSpace(choice)
		if !seen[choice] {
			choices = append(choices, choice)
			seen[choice] = true
		}
	}
	return choices
}

func sameChoices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	set := make(map[string]bool, len(right))
	for _, choice := range right {
		set[choice] = true
	}
	for _, choice := range left {
		if !set[choice] {
			return false
		}
	}
	return true
}
