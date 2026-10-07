package check

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

func checkWords(snapshot *spec.Snapshot, args ...string) Result {
	example := extract.Example{}
	for _, arg := range args {
		example.Words = append(example.Words, extract.Word{Value: arg, Static: true})
	}
	return Example(example, snapshot)
}

func FuzzAttachedValues(f *testing.F) {
	for _, value := range []string{"", "file", "-", "--unknown", "a=b", "=x", "é", " ", "\x00"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 4096 {
			t.Skip()
		}
		snapshot := helpFixture()
		separate := checkWords(snapshot, "acme", "publish", "--output", value)
		attached := checkWords(snapshot, "acme", "publish", "--output="+value)
		if !reflect.DeepEqual(attached, separate) {
			t.Fatalf("attached value %q changed verdict:\nattached: %+v\nseparate: %+v", value, attached, separate)
		}
	})
}

// Unknown arity, positional/forwarded tails and expansions cannot justify new claims.
func FuzzOpaqueTails(f *testing.F) {
	for _, tail := range []string{"--output", "publish\x00--unknown", "", "--output\x00$DYNAMIC"} {
		f.Add(tail)
	}
	f.Fuzz(func(t *testing.T, tail string) {
		if len(tail) > 4096 {
			t.Skip()
		}
		for _, head := range []extract.Word{
			{Value: "--", Static: true}, {Value: "script.js", Static: true},
			{Value: "--unknown", Static: true}, {Value: "--switch", Static: true},
			{Value: "$DYNAMIC", Static: false},
		} {
			snapshot := helpFixture()
			example := extract.Example{Words: []extract.Word{
				{Value: "acme", Static: true}, {Value: "publish", Static: true}, head,
			}}
			want := Example(example, snapshot)
			want.Diagnostics = slices.DeleteFunc(want.Diagnostics, func(d Diagnostic) bool { return d.Status == "uncheckable" })
			for arg := range strings.SplitSeq(tail, "\x00") {
				example.Words = append(example.Words, extract.Word{Value: arg, Static: true})
			}
			got := Example(example, snapshot)
			// A stopped check may become visible, but opaque words cannot produce
			// new claims that a flag/argument is valid or invalid.
			got.Diagnostics = slices.DeleteFunc(got.Diagnostics, func(d Diagnostic) bool { return d.Status == "uncheckable" })
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("opaque tail %q after %q changed verdict:\ngot: %+v\nwant: %+v", tail, head.Value, got, want)
			}
		}
	})
}
