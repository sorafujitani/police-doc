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
	// Scalar means an unevaluated expression still contributes exactly one word.
	Scalar bool
}

type Example struct {
	Location        Location
	Text            string
	Words           []Word
	LocalPackage    bool // npx: prefer an installed project-local executable when collecting help.
	Reason          string
	Code            string
	HasSubstitution bool
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
		if language != "sh" && language != "bash" && language != "shell" && language != "console" && language != "" {
			return ast.WalkSkipChildren, nil
		}
		var lines []string
		var origins []origin
		hasPrompt, promptStyleKnown, ambiguousInput := false, false, false
		for i := range block.Lines().Len() {
			segment := block.Lines().At(i)
			line := string(segment.Value(source))
			lines = append(lines, line)
			origins = append(origins, origin{offset: segment.Start, padding: segment.Padding})
			trimmed := strings.TrimSpace(line)
			if !promptStyleKnown && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				hasPrompt = strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "❯ ")
				// A leading '>' could be either a prompt (fd's README) or a
				// shell redirection. Never interpret its output as commands.
				ambiguousInput = strings.HasPrefix(trimmed, "> ")
				if language == "console" {
					label, _, _ := strings.Cut(trimmed, ":")
					switch strings.ToLower(label) {
					case "warning", "error", "fatal", "usage":
						ambiguousInput = true // Diagnostic-only terminal output.
					}
				}
				promptStyleKnown = true
			}
		}
		if language == "" && !hasPrompt {
			return ast.WalkSkipChildren, nil // Do not guess the language of unlabelled data.
		}
		body, ambiguous := shellBody(lines, origins, hasPrompt)
		ambiguous = ambiguous || ambiguousInput
		parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
		file, err := parser.Parse(strings.NewReader(body), path)
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
		if err != nil {
			location := Location{File: path, Line: 1, Column: 1}
			if parseError, ok := errors.AsType[syntax.ParseError](err); ok {
				location = locate(parseError.Pos)
			}
			code, reason := "invalid-shell", err.Error()
			if ambiguous {
				code, reason = "unsupported-shell", "Terminal output cannot be distinguished from multiline shell data: "+reason
			}
			examples = append(examples, Example{Location: location, Text: strings.TrimSpace(body), Code: code, Reason: reason})
			return ast.WalkSkipChildren, nil
		}
		if ambiguous {
			examples = append(examples, Example{Location: locate(file.Pos()), Text: strings.TrimSpace(body), Code: "unsupported-shell", Reason: "Shell input and terminal output are ambiguous; no commands were extracted."})
			return ast.WalkSkipChildren, nil
		}
		bindings := shellBindings(file)
		substitutions := make(map[*syntax.CallExpr]bool)
		handledSubstitutions := make(map[syntax.Node]bool)
		markSubstitutions := func(node syntax.Node) bool {
			found := false
			syntax.Walk(node, func(node syntax.Node) bool {
				switch node.(type) {
				case *syntax.CmdSubst, *syntax.ProcSubst:
					handledSubstitutions[node], found = true, true
					return false
				}
				return true
			})
			return found
		}
		recordSubstitutionGap := func(node syntax.Node) {
			examples = append(examples, Example{Location: locate(node.Pos()), Text: nodeText(body, node), Code: "unchecked-substitution", Reason: "Commands inside a substitution were not checked; no substitution was executed."})
		}
		syntax.Walk(file, func(node syntax.Node) bool {
			switch node.(type) {
			case *syntax.CmdSubst, *syntax.ProcSubst:
				if !handledSubstitutions[node] {
					recordSubstitutionGap(node)
				}
				return false // No nested command is evaluated or invoked.
			}
			if statement, ok := node.(*syntax.Stmt); ok {
				switch command := statement.Cmd.(type) {
				case *syntax.CallExpr:
					substitutions[command] = markSubstitutions(statement)
				case *syntax.DeclClause:
					example := declarationExample(body, command, locate(command.Pos()))
					example.HasSubstitution = markSubstitutions(statement)
					examples = append(examples, example)
					return false
				case nil:
					if markSubstitutions(statement) {
						recordSubstitutionGap(statement)
					}
				case *syntax.BinaryCmd, *syntax.Block, *syntax.Subshell, *syntax.IfClause,
					*syntax.WhileClause, *syntax.ForClause, *syntax.CaseClause, *syntax.FuncDecl, *syntax.TimeClause:
					// Inspect bodies independently of whether control flow would run them.
					for _, redirect := range statement.Redirs {
						if markSubstitutions(redirect) {
							recordSubstitutionGap(redirect)
						}
					}
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
				if substitutions[call] {
					recordSubstitutionGap(call)
				}
				return false // Plain assignments are not CLI examples; substitutions still leave a gap.
			}
			example := Example{Location: locate(call.Args[0].Pos()), Text: nodeText(body, call), HasSubstitution: substitutions[call]}
			for _, arg := range call.Args {
				syntax.SplitBraces(arg)
				value, static := literal(arg.Parts, false)
				example.Words = append(example.Words, Word{Value: value, Static: static, Scalar: !static && scalar(arg.Parts, false)})
			}
			for _, assignment := range call.Assigns {
				if assignment.Name != nil && assignment.Name.Value == "PATH" {
					example.Code, example.Reason = "unsupported-environment", "The example changes PATH; it was not applied, so the executable and its arguments were not checked."
				}
			}
			if len(example.Words) > 0 && example.Words[0].Static && strings.HasPrefix(example.Words[0].Value, "-") {
				example.Code, example.Reason = "argument-fragment", "This block starts with options rather than an executable; its owning CLI or configuration format must be specified."
			}
			if len(example.Words) > 0 && example.Words[0].Static && bindings[example.Words[0].Value] {
				example.Code, example.Reason = "shell-binding", "This name is defined as a shell function or alias in the document; an unrelated installed executable was not used."
			}
			examples = append(examples, unwrap(example))
			return false // Do not treat nested substitutions as independent examples.
		})
		return ast.WalkSkipChildren, nil
	})
	return examples
}

func containsSubstitution(node syntax.Node) bool {
	found := false
	syntax.Walk(node, func(node syntax.Node) bool {
		switch node.(type) {
		case *syntax.CmdSubst, *syntax.ProcSubst:
			found = true
		}
		return !found
	})
	return found
}

func nodeText(body string, node syntax.Node) string {
	start, end := int(node.Pos().Offset()), int(node.End().Offset())
	if start < 0 || end > len(body) || start > end {
		return ""
	}
	return strings.TrimSpace(body[start:end])
}

// Scalar shape is independent of value: quoted substitutions do not undergo
// word splitting. "$@" and unquoted expansions can still contribute many words.
func scalar(parts []syntax.WordPart, quoted bool) bool {
	for _, part := range parts {
		switch part := part.(type) {
		case *syntax.Lit:
			if !quoted && strings.ContainsAny(part.Value, "*?[") {
				return false
			}
		case *syntax.SglQuoted:
		case *syntax.DblQuoted:
			if part.Dollar || !scalar(part.Parts, true) {
				return false
			}
		case *syntax.ParamExp:
			if !quoted || part.Param == nil || part.Param.Value == "@" || part.Index != nil || part.Excl ||
				part.Exp != nil || part.Slice != nil || part.Repl != nil || part.Names != 0 ||
				part.NestedParam != nil || part.Flags != nil || len(part.Modifiers) != 0 {
				return false
			}
		case *syntax.CmdSubst:
			if !quoted {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func literal(parts []syntax.WordPart, quoted bool) (string, bool) {
	if len(parts) == 1 {
		if part, ok := parts[0].(*syntax.Lit); ok && part.Value == "[" {
			return "[", true
		}
	}
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
