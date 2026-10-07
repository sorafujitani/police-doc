package spec

import (
	"regexp"
	"strings"
)

var (
	helpChoiceSet       = regexp.MustCompile(`([{<])([A-Za-z0-9_.:/+-]+(?:[ \t]*[|,][ \t]*[A-Za-z0-9_.:/+-]+)+)([}>])`)
	helpExplicitChoices = regexp.MustCompile(`^(?:choices|(?:allowed|possible|accepted|valid|supported) values|(?:must be )?one of)(?: \(default [^():]+\))?$`)
)

// Require an enumeration in a signature or a labeled value constraint, not an
// arbitrary colon in prose. Angle-bracket tuples and metavariables are not enums.
func helpFlagChoices(name, signature, description string) []string {
	// Later usage items may belong to positionals, not this flag.
	items := helpUsageItems(strings.Fields(strings.TrimPrefix(signature, "=")))
	if len(items) > 0 {
		signature = items[0]
	}
	match := helpChoiceSet.FindStringSubmatch(signature)
	explicit := false
	if match == nil {
		if label, text, found := strings.Cut(description, ":"); found {
			label = strings.ToLower(strings.TrimSpace(label))
			explicit = helpExplicitChoices.MatchString(label)
			if strings.Contains(label, "example") || (strings.Contains(label, "default") && !explicit) {
				return nil // Examples and defaults do not restrict allowed values.
			}
			field := strings.TrimLeft(strings.ReplaceAll(name, "[no-]", ""), "-")
			field = strings.ToLower(strings.NewReplacer("-", " ", "_", " ").Replace(field))
			if !explicit && strings.TrimPrefix(label, "filter by ") != field {
				return nil
			}
			text = strings.TrimSpace(text)
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
		if match[1] == "<" && !explicit && (strings.Contains(match[2], ",") || helpMetavar.MatchString(choice)) {
			return nil
		}
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
