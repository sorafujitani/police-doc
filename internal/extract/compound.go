package extract

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// A declaration is shell syntax, not a reason to discard an entire block. Its
// arguments still go through the builtin checker; no assignment is applied.
func declarationExample(body string, node *syntax.DeclClause, location Location) Example {
	example := Example{Location: location, Text: nodeText(body, node), HasSubstitution: containsSubstitution(node)}
	example.Words = append(example.Words, Word{Value: node.Variant.Value, Static: true})
	for _, assignment := range node.Args {
		if assignment.Index != nil || assignment.Array != nil {
			example.Code, example.Reason = "unsupported-shell", "Array declarations need shell-specific argument rules; no declaration was executed."
			break
		}
		word := Word{Static: true}
		if assignment.Value != nil {
			word.Value, word.Static = literal(assignment.Value.Parts, assignment.Name != nil)
			word.Scalar = !word.Static && scalar(assignment.Value.Parts, assignment.Name != nil)
		}
		if assignment.Name != nil {
			prefix := assignment.Name.Value
			if !assignment.Naked {
				if assignment.Append {
					prefix += "+"
				}
				prefix += "="
			}
			word.Value = prefix + word.Value
			if assignment.Name.Value == "PATH" && !assignment.Naked {
				example.Code, example.Reason = "unsupported-environment", "The example changes PATH; that environment was not applied to metadata collection."
			}
		}
		example.Words = append(example.Words, word)
	}
	return unwrap(example)
}

// Bindings in the document must not accidentally select an unrelated installed
// executable. This is conservative across subshell scopes; it never evaluates
// an alias or calls a user-defined function to discover its behavior.
func shellBindings(file *syntax.File) map[string]bool {
	bindings := make(map[string]bool)
	syntax.Walk(file, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.FuncDecl:
			bindings[node.Name.Value] = true
		case *syntax.CallExpr:
			if len(node.Args) > 0 {
				name, ok := literal(node.Args[0].Parts, false)
				if ok && name == "alias" {
					for _, arg := range node.Args[1:] {
						value, static := literal(arg.Parts, false)
						if key, _, ok := strings.Cut(value, "="); static && ok {
							bindings[key] = true
						}
					}
				}
			}
		}
		return true
	})
	return bindings
}
