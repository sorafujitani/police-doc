package check

import "testing"

func TestEveryIncompleteResultHasRequirementsWithoutBecomingChecked(t *testing.T) {
	for _, code := range []string{"missing-reference", "builtin-help-unavailable", "unknown-flag-arity", "unverified-arguments", "dynamic-argument", "unchecked-substitution", "shell-binding", "argument-fragment", "invalid-shell", "new-unsupported-rule"} {
		report := NewReport(1)
		report.RequireComplete = true
		report.Add(Result{Coverage: "partial", Diagnostics: []Diagnostic{{Code: code, Severity: "info", Status: "uncheckable"}}})
		if report.CoverageComplete || report.Summary.Checked != 0 || len(report.Results[0].Requirements) != 1 {
			t.Fatalf("gap was hidden: %+v", report)
		}
		need := report.Results[0].Requirements[0]
		if need.Code != code || need.Kind == "" || need.Action == "" {
			t.Fatalf("unhandled gap: %+v", need)
		}
	}
	report := NewReport(1)
	report.Add(Result{Coverage: "checked", Diagnostics: []Diagnostic{{Code: "missing-flag-value", Status: "confirmed", Severity: "error"}}})
	if len(report.Results[0].Requirements) != 0 || !report.Fails("error") {
		t.Fatal("an invalid but checked command became a missing-evidence case")
	}
}
