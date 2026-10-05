package extract

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"mvdan.cc/sh/v3/syntax"
)

type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Word struct {
	Value  string
	Static bool
}

type Example struct {
	Location     Location
	Text         string
	Words        []Word
	LocalPackage bool // npx: prefer an installed project-local executable when collecting help.
	Reason       string
	Code         string
}

type origin struct {
	offset     int
	padding    int
	removed    int
	bodyOffset int
}

// Markdown only parses source text. It never expands or executes shell words.
func Markdown(path string, source []byte) []Example {
	starts := []int{0}
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	document := goldmark.New().Parser().Parse(text.NewReader(source))
	var examples []Example
	columnOffset, column := 0, 1
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		block, ok := node.(*ast.FencedCodeBlock)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		language := string(block.Language(source))
		if language != "sh" && language != "bash" && language != "console" {
			return ast.WalkSkipChildren, nil
		}
		var lines []string
		var origins []origin
		hasPrompt, promptStyleKnown := false, false
		for i := range block.Lines().Len() {
			segment := block.Lines().At(i)
			line := string(segment.Value(source))
			lines = append(lines, line)
			origins = append(origins, origin{offset: segment.Start, padding: segment.Padding})
			trimmed := strings.TrimSpace(line)
			if !promptStyleKnown && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				hasPrompt = strings.HasPrefix(strings.TrimLeft(line, " \t"), "$ ")
				promptStyleKnown = true
			}
		}
		var code strings.Builder
		discardedOutput := false
		for i, line := range lines {
			trimmed := strings.TrimLeft(line, " \t")
			indent := len(line) - len(trimmed)
			switch {
			case hasPrompt && strings.HasPrefix(trimmed, "$ "):
				origins[i].removed = indent + 2
				line = trimmed[2:]
			case strings.HasPrefix(trimmed, "> ") && hasPrompt:
				origins[i].removed = indent + 2
				line = trimmed[2:]
			case language == "console" && hasPrompt:
				discardedOutput = discardedOutput || strings.TrimSpace(line) != ""
				line = "\n" // Preserve line numbers while dropping terminal output.
			}
			origins[i].bodyOffset = code.Len()
			code.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				code.WriteByte('\n')
			}
		}
		body := code.String()
		locate := func(pos syntax.Pos) Location {
			if len(origins) == 0 {
				return Location{File: path, Line: 1, Column: 1}
			}
			// Pos.Line/Col overflow on large inputs; byte offsets remain valid.
			bodyOffset := int(pos.Offset())
			index := max(0, sort.Search(len(origins), func(i int) bool { return origins[i].bodyOffset > bodyOffset })-1)
			o := origins[index]
			offset := min(len(source), max(0, o.offset+o.removed+bodyOffset-o.bodyOffset-o.padding))
			line := sort.Search(len(starts), func(i int) bool { return starts[i] > offset }) - 1
			// AST locations advance through the source: count each rune only once.
			if columnOffset < starts[line] || columnOffset > offset {
				columnOffset, column = starts[line], 1
			}
			column += utf8.RuneCount(source[columnOffset:offset])
			columnOffset = offset
			return Location{File: path, Line: line + 1, Column: column}
		}
		parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
		file, err := parser.Parse(strings.NewReader(body), path)
		if err != nil {
			location := Location{File: path, Line: 1, Column: 1}
			if parseError, ok := errors.AsType[syntax.ParseError](err); ok {
				location = locate(parseError.Pos)
			}
			code, reason := "invalid-shell", err.Error()
			if discardedOutput {
				code, reason = "unsupported-shell", "Console output cannot be distinguished from multiline shell data: "+reason
			}
			examples = append(examples, Example{Location: location, Text: strings.TrimSpace(body), Code: code, Reason: reason})
			return ast.WalkSkipChildren, nil
		}
		if hasPrompt {
			// Without a terminal transcript grammar, prompt-like literal data is
			// ambiguous. Reject the block instead of turning that data into commands.
			ambiguous := false
			syntax.Walk(file, func(node syntax.Node) bool {
				switch node := node.(type) {
				case *syntax.Redirect:
					ambiguous = ambiguous || node.Op == syntax.Hdoc || node.Op == syntax.DashHdoc
				case *syntax.Word:
					ambiguous = ambiguous || strings.Contains(nodeText(body, node), "\n")
				}
				return !ambiguous
			})
			if ambiguous {
				examples = append(examples, Example{Location: locate(file.Pos()), Text: strings.TrimSpace(body), Code: "unsupported-shell", Reason: "Prompted multiline words and here-documents are ambiguous; no commands were extracted."})
				return ast.WalkSkipChildren, nil
			}
		}
		syntax.Walk(file, func(node syntax.Node) bool {
			switch node.(type) {
			case *syntax.CmdSubst, *syntax.ProcSubst:
				return false // Substitutions are never independent examples, including in redirects.
			}
			if statement, ok := node.(*syntax.Stmt); ok {
				switch statement.Cmd.(type) {
				case nil, *syntax.CallExpr, *syntax.BinaryCmd:
				default:
					examples = append(examples, Example{Location: locate(statement.Pos()), Text: nodeText(body, statement), Code: "unsupported-shell", Reason: "Compound shell statements are not supported."})
					return false
				}
			}
			call, ok := node.(*syntax.CallExpr)
			if !ok {
				return true
			}
			if len(call.Args) == 0 {
				return false // An assignment alone is not a CLI example.
			}
			example := Example{Location: locate(call.Args[0].Pos()), Text: nodeText(body, call)}
			for _, arg := range call.Args {
				syntax.SplitBraces(arg)
				value, static := literal(arg.Parts, false)
				example.Words = append(example.Words, Word{Value: value, Static: static})
			}
			examples = append(examples, unwrap(example))
			return false // Do not treat nested substitutions as independent examples.
		})
		return ast.WalkSkipChildren, nil
	})
	return examples
}

func nodeText(body string, node syntax.Node) string {
	start, end := int(node.Pos().Offset()), int(node.End().Offset())
	if start < 0 || end > len(body) || start > end {
		return ""
	}
	return strings.TrimSpace(body[start:end])
}

func literal(parts []syntax.WordPart, quoted bool) (string, bool) {
	var value strings.Builder
	for _, part := range parts {
		switch part := part.(type) {
		case *syntax.Lit:
			for i := 0; i < len(part.Value); i++ {
				b := part.Value[i]
				if b == '\\' && i+1 < len(part.Value) {
					next := part.Value[i+1]
					if !quoted || strings.ContainsRune("$`\"\\\n", rune(next)) {
						i++
						if next != '\n' {
							value.WriteByte(next)
						}
						continue
					}
				}
				if !quoted && (strings.ContainsRune("*?[", rune(b)) || (b == '~' && value.Len() == 0)) {
					return "", false
				}
				value.WriteByte(b)
			}
		case *syntax.SglQuoted:
			if part.Dollar {
				return "", false
			}
			value.WriteString(part.Value)
		case *syntax.DblQuoted:
			if part.Dollar {
				return "", false
			}
			nested, static := literal(part.Parts, true)
			if !static {
				return "", false
			}
			value.WriteString(nested)
		default:
			return "", false
		}
	}
	return value.String(), true
}
