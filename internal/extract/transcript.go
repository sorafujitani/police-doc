package extract

import (
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shellBody treats every supported fence consistently. The shell parser identifies
// each complete prompted command; the remaining lines are terminal output.
func shellBody(lines []string, origins []origin, prompted bool) (string, bool) {
	var prompts []int
	for i, line := range lines {
		if prompted {
			trimmed := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "> ") {
				origins[i].removed = len(line) - len(trimmed) + 2
				line = trimmed[2:]
				if strings.HasPrefix(trimmed, "$ ") {
					prompts = append(prompts, i)
				}
			}
		}
		if !strings.HasSuffix(line, "\n") {
			line += "\n"
		}
		lines[i] = line
	}
	ambiguous := false
	if len(prompts) > 0 {
		for i := range prompts[0] {
			lines[i] = "\n"
		}
		prompts = append(prompts, len(lines))
		for i, start := range prompts[:len(prompts)-1] {
			end := prompts[i+1]
			input := strings.Join(lines[start:end], "")
			keep := end - start
			parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
			incomplete := false
			for statements, err := range parser.InteractiveSeq(strings.NewReader(input)) {
				if err != nil {
					// A new prompt inside incomplete input may actually be literal data.
					ambiguous = ambiguous || ((incomplete || syntax.IsIncomplete(err)) && (end < len(lines) || end-start > 1))
					if parseError, ok := errors.AsType[syntax.ParseError](err); ok {
						line := start + strings.Count(input[:parseError.Pos.Offset()], "\n")
						// Malformed unprompted text may be output, not shell input.
						ambiguous = ambiguous || (line > start && line < end && origins[line].removed == 0)
					}
					break // Keep malformed input for the main parser's source diagnostic.
				}
				if parser.Incomplete() {
					incomplete = true
					continue
				}
				keep = 1
				for _, statement := range statements {
					keep = max(keep, strings.Count(input[:statement.End().Offset()], "\n")+1)
					syntax.Walk(statement, func(node syntax.Node) bool {
						switch node := node.(type) {
						case *syntax.Redirect:
							ambiguous = ambiguous || node.Op == syntax.Hdoc || node.Op == syntax.DashHdoc
						case *syntax.Word:
							ambiguous = ambiguous || strings.Contains(nodeText(input, node), "\n")
						}
						return !ambiguous
					})
				}
				break // A complete input line was parsed; the rest is terminal output.
			}
			for j := start + keep; j < end; j++ {
				lines[j] = "\n"
			}
		}
	}
	var body strings.Builder
	for i, line := range lines {
		origins[i].bodyOffset = body.Len()
		body.WriteString(line)
	}
	return body.String(), ambiguous
}
