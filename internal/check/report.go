package check

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

type Diagnostic struct {
	Severity string          `json:"severity"`
	Status   string          `json:"status"`
	Code     string          `json:"code"`
	Message  string          `json:"message"`
	Detail   string          `json:"detail,omitempty"`
	Evidence []spec.Evidence `json:"evidence"`
}

type Result struct {
	// HelpPath requests missing help for an advertised command, not document argv.
	HelpPath    []string         `json:"-"`
	Location    extract.Location `json:"location"`
	Command     string           `json:"command"`
	CLI         string           `json:"cli,omitempty"`
	Version     string           `json:"version,omitempty"`
	OS          string           `json:"os,omitempty"`
	Coverage    string           `json:"coverage"`
	Diagnostics []Diagnostic     `json:"diagnostics"`
}

type Summary struct {
	Files       int `json:"files"`
	Examples    int `json:"examples"`
	Partial     int `json:"partial"`
	Uncheckable int `json:"uncheckable"`
	Errors      int `json:"errors"`
	Warnings    int `json:"warnings"`
}

type Report struct {
	SchemaVersion int               `json:"schema_version"`
	Environment   map[string]string `json:"environment"`
	Summary       Summary           `json:"summary"`
	Results       []Result          `json:"results"`
}

func NewReport(files int) *Report {
	return &Report{
		SchemaVersion: 2,
		Environment:   map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH},
		Summary:       Summary{Files: files},
		Results:       []Result{},
	}
}

func (report *Report) Add(result Result) {
	report.Results = append(report.Results, result)
	report.Summary.Examples++
	switch result.Coverage {
	case "partial":
		report.Summary.Partial++
	case "uncheckable":
		report.Summary.Uncheckable++
	}
	for _, diagnostic := range result.Diagnostics {
		switch diagnostic.Severity {
		case "error":
			report.Summary.Errors++
		case "warning":
			report.Summary.Warnings++
		}
	}
}

func (report *Report) Fails(threshold string) bool {
	return threshold != "none" && (report.Summary.Errors > 0 || (threshold == "warning" && report.Summary.Warnings > 0))
}

func (report *Report) Write(writer io.Writer, format string, verbose bool) error {
	if format == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	var text strings.Builder
	for _, result := range report.Results {
		shown := false
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Severity == "info" && !verbose {
				continue
			}
			if !shown {
				fmt.Fprintf(&text, "%s:%d:%d\n  %s\n", terminalText(result.Location.File), result.Location.Line, result.Location.Column, terminalText(result.Command))
				if verbose && result.CLI != "" {
					fmt.Fprintf(&text, "  Target: %s %s (os=%s)\n", terminalText(result.CLI), terminalText(result.Version), terminalText(cmp.Or(result.OS, "any")))
				}
				shown = true
			}
			fmt.Fprintf(&text, "  %s %s", terminalText(strings.ToUpper(diagnostic.Severity)), terminalText(diagnostic.Code))
			if verbose {
				fmt.Fprintf(&text, " [%s]", terminalText(diagnostic.Status))
			}
			fmt.Fprintf(&text, ": %s\n", terminalText(diagnostic.Message))
			if verbose {
				if diagnostic.Detail != "" {
					fmt.Fprintf(&text, "    Details: %s\n", terminalText(diagnostic.Detail))
				}
				for _, evidence := range diagnostic.Evidence {
					fmt.Fprintf(&text, "    Evidence (%s): %s", terminalText(evidence.Kind), terminalText(evidence.Reference))
					if evidence.Detail != "" {
						fmt.Fprintf(&text, " — %s", terminalText(evidence.Detail))
					}
					text.WriteByte('\n')
				}
			}
		}
		if shown {
			text.WriteByte('\n')
		}
	}
	s := report.Summary
	fmt.Fprintf(&text, "%d files, %d examples: %d errors, %d warnings\n", s.Files, s.Examples, s.Errors, s.Warnings)
	if s.Examples == 0 {
		text.WriteString("No supported shell examples found.\n")
	} else {
		fmt.Fprintf(&text, "Coverage: %d partially checked, %d uncheckable.", s.Partial, s.Uncheckable)
		if !verbose {
			text.WriteString(" Use --verbose for details.")
		}
		text.WriteByte('\n')
	}
	_, err := io.WriteString(writer, text.String())
	return err
}

// terminalText keeps untrusted data from controlling the terminal or forging lines.
// JSON output retains the original strings through normal JSON escaping.
func terminalText(value string) string {
	var text strings.Builder
	for _, r := range value {
		if unicode.IsPrint(r) {
			text.WriteRune(r)
		} else {
			quoted := strconv.QuoteRune(r)
			text.WriteString(quoted[1 : len(quoted)-1])
		}
	}
	return text.String()
}
