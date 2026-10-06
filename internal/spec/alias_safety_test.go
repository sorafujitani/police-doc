package spec

import "testing"

func TestAliasesRequireCompleteNonconflictingCatalog(t *testing.T) {
	for _, tc := range []struct {
		name, betaAlias string
		budget          int
		complete, found bool
	}{
		{"unique", "y", 3, true, true},
		{"conflicting", "x", 3, true, false},
		{"incomplete", "y", 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("POLICEDOC_TEST_BETA_ALIAS", tc.betaAlias)
			binary := helpExecutable(t, `
case "$*" in
  -h) printf 'Usage: acme COMMAND\nCommands:\n  alpha  First command\n  beta  Second command\n' ;;
  'alpha -h') printf 'Usage: acme alpha\n\nAliases:\n  x\n\nFlags:\n  --only-alpha\n' ;;
  'beta -h') printf 'Usage: acme beta\n\nAliases:\n  %s\n' "$POLICEDOC_TEST_BETA_ALIAS" ;;
  *) exit 9 ;;
esac
`)
			collector, err := NewHelpCollector(t.Context(), HelpOptions{Binary: binary, Version: "1.0.0", MaxCommands: tc.budget}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.budget > 1 {
				if err := collector.Collect(t.Context(), []string{"alpha"}); err != nil {
					t.Fatal(err)
				}
				if collector.Snapshot.Root.Child("x") != nil {
					t.Fatal("one collected sibling prematurely established a unique alias")
				}
			}
			if err := collector.CollectAliases(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
			root := &collector.Snapshot.Root
			if root.AliasesComplete != tc.complete || (root.Child("x") != nil) != tc.found {
				t.Fatalf("unexpected alias resolution: %+v", root)
			}
		})
	}
}

func TestShortFlagSyntaxIsNotAssumedForSingleDashFormats(t *testing.T) {
	for _, tc := range []struct {
		help string
		want bool
	}{
		{"Options:\n  -i, --ignore-case\n  -n, --line-number\n", true},
		{"Options:\n  -n\n  -v\n  -race\n", false},
		{"Options:\n  -n\n  -v\n", false},
		{"Options:\n  -v, --verbose\n  -version\n", false},
	} {
		if command := parseHelp(tc.help); command.ShortFlagClusters != tc.want {
			t.Errorf("cluster syntax=%v, want %v for %q", command.ShortFlagClusters, tc.want, tc.help)
		}
	}
}
