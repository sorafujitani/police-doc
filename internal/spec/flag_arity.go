package spec

import "strings"

// Count explicit value slots without counting operands outside a usage bracket.
// Optional or variadic tails do not establish a fixed arity.
func helpValueArity(tail string) (string, int) {
	mode := helpValue(tail)
	if mode != "required" {
		return mode, 0
	}
	items := helpUsageItems(strings.Fields(strings.TrimPrefix(tail, "=")))
	// A single attached placeholder (including "...") still describes one value.
	if len(items) == 1 && strings.HasPrefix(tail, "=") {
		return mode, 0
	}
	count := 0
	for _, item := range items {
		if item == "[" || strings.HasPrefix(item, "]") {
			break
		}
		marker := strings.TrimRight(item, "],")
		if strings.Contains(marker, "...") || strings.HasPrefix(marker, "[") {
			return "unknown", 0
		}
		if helpValue(" "+marker) != "required" {
			return "unknown", 0
		}
		count++
		if strings.HasSuffix(item, "]") {
			break
		}
	}
	if count <= 1 {
		return mode, 0
	}
	return mode, count
}
