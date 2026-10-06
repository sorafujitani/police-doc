package spec

import (
	"regexp"
	"strings"
)

var (
	helpOptionHeading = regexp.MustCompile(`(?i)^(?:[a-z][a-z -]*\s+)?(?:options?|flags?):?$`)
	helpTitleHeading  = regexp.MustCompile(`^[A-Z][a-zA-Z-]*(?:[ \t]+[A-Z][a-zA-Z-]*)*:$`)
	helpPairedFlagRow = regexp.MustCompile(`^-[A-Za-z0-9](?:\.\.\.)?\s*[,|/]\s*--[A-Za-z0-9]`)
)

// Separate option declarations from examples, prose, and nested descriptions.
// A section uses either aligned names (including man pages), or getopt columns
// with long-only names four spaces after rows that have short aliases.
func helpFlagLines(lines []string) map[int]bool {
	type row struct {
		line, indent int
		long, paired bool
	}
	allowed := make(map[int]bool)
	var rows []row
	section, usage := true, false
	declarationIndent, nestedHeading := -1, -1
	flush := func() {
		declarationIndent = -1
		if len(rows) == 0 {
			return
		}
		base := rows[0].indent
		for _, row := range rows {
			base = min(base, row.indent)
		}
		shortAtBase, longAtBase := false, false
		for _, row := range rows {
			if row.indent == base {
				shortAtBase = shortAtBase || !row.long
				longAtBase = longAtBase || row.long
			}
		}
		for _, row := range rows {
			if row.indent == base || row.paired || (shortAtBase && !longAtBase && row.long && row.indent == base+4) {
				allowed[row.line] = true
			}
		}
		rows = nil
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			usage = false
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if nestedHeading >= 0 {
			if indent > nestedHeading {
				continue
			}
			nestedHeading = -1
		}
		if helpUsage.MatchString(line) {
			flush()
			section, usage = true, true
			allowed[i] = true
			continue
		}
		if helpOptionHeading.MatchString(trimmed) && (declarationIndent < 0 || indent <= declarationIndent) {
			flush()
			section, usage = true, false
			continue
		}
		if !strings.HasPrefix(trimmed, "-") {
			if indent == 0 && (helpUpperHeading.MatchString(trimmed) || helpTitleHeading.MatchString(trimmed) || helpCommandHeading.MatchString(trimmed)) {
				flush()
				section, usage = false, false
				continue
			}
			if indent > 0 && strings.HasSuffix(trimmed, ":") && (declarationIndent >= 0 || strings.Contains(strings.ToLower(trimmed), "example")) {
				nestedHeading = indent
				continue
			}
		}
		if !section {
			continue
		}
		if strings.HasPrefix(trimmed, "[-") || (usage && strings.Contains(trimmed, "[-")) {
			allowed[i] = true
			continue
		}
		match := helpFlagName.FindStringIndex(trimmed)
		if match == nil || match[0] != 0 {
			continue
		}
		if declarationIndent < 0 || indent < declarationIndent {
			declarationIndent = indent
		}
		rows = append(rows, row{line: i, indent: indent, long: strings.HasPrefix(trimmed, "--"), paired: helpPairedFlagRow.MatchString(trimmed)})
	}
	flush()
	return allowed
}
