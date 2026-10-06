package spec

import (
	"path/filepath"
	"strings"
)

// Keep wrapped lines in one synopsis, but keep alternative invocations separate.
func helpSynopses(output string) [][]string {
	primary := helpUsage.MatchString(output)
	var synopses [][]string
	var words []string
	usage := false
	flush := func() {
		if len(words) > 0 {
			synopses = append(synopses, words)
			words = nil
		}
	}
	for line := range strings.SplitSeq(output, "\n") {
		if match := helpUsage.FindStringIndex(line); match != nil {
			flush()
			usage = true
			line = line[match[1]:]
		} else {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				if len(words) > 0 {
					flush()
					usage = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "-") {
				flush() // An option definition is not a wrapped usage line.
				usage = false
				continue
			}
			if strings.HasSuffix(trimmed, ":") || helpUpperHeading.MatchString(trimmed) {
				flush()
				heading := strings.ToLower(strings.TrimSuffix(trimmed, ":"))
				usage = !primary && (heading == "example usage" || heading == "further help")
				continue
			}
		}
		if !usage {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(words) > 0 && filepath.Base(fields[0]) == filepath.Base(words[0]) {
			flush()
		}
		words = append(words, fields...)
	}
	flush()
	return synopses
}

func helpUsageItems(words []string) []string {
	var items []string
	start, depth := 0, 0
	for i, word := range words {
		for _, char := range word {
			switch char {
			case '[', '<', '(', '{':
				depth++
			case ']', '>', ')', '}':
				depth--
			}
		}
		if depth <= 0 {
			items = append(items, strings.Join(words[start:i+1], " "))
			start, depth = i+1, 0
		}
	}
	if start < len(words) {
		items = append(items, strings.Join(words[start:], " "))
	}
	return items
}

func helpOptionsItem(item string) bool {
	if strings.EqualFold(item, "[options]") || strings.EqualFold(item, "[flags]") {
		return true
	}
	if !strings.HasPrefix(item, "[-") || !strings.HasSuffix(item, "]") {
		return false
	}
	// Every alternative must start with a flag, not a positional placeholder.
	for branch := range strings.SplitSeq(item[1:len(item)-1], "|") {
		if !strings.HasPrefix(strings.TrimSpace(branch), "-") {
			return false
		}
	}
	return true
}

func helpCommandItem(item string) bool {
	switch strings.ToLower(item) {
	case "command", "<command>", "[command]", "subcommand", "<subcommand>", "[subcommand]":
		return true
	}
	return false
}

// A trailing flags/options marker explicitly documents flags after operands.
// Never infer this for command forwarding or ambiguous flag/operand alternatives.
func helpFlagsAfterPositionals(words []string) bool {
	positional, trailingFlags := false, false
	for _, item := range helpUsageItems(words) {
		if helpCommandItem(item) || strings.Contains(item, "-- ") {
			return false
		}
		if helpOptionsItem(item) {
			if positional && (strings.EqualFold(item, "[flags]") || strings.EqualFold(item, "[options]")) {
				trailingFlags = true
			}
			continue
		}
		positional = true
	}
	return trailingFlags
}
