package types

import (
	"strings"
	"testing"
)

// FuzzNormalizedKey_matchesConcatLower locks NormalizedKey to strings.ToLower on
// the legacy concat form (same contract as TestNormalizedKeyUnicodeAgainstConcatReference).
func FuzzNormalizedKey_matchesConcatLower(f *testing.F) {
	f.Add("Reporting", "Views", "Monthly")
	f.Add("", "", "")
	f.Fuzz(func(t *testing.T, schema, kind, name string) {
		const max = 256
		if len(schema) > max || len(kind) > max || len(name) > max {
			t.Skip()
		}
		want := strings.ToLower(schema + "/" + kind + "/" + name)
		if got := NormalizedKey(schema, kind, name); got != want {
			t.Fatalf("NormalizedKey(%q,%q,%q) = %q, want %q", schema, kind, name, got, want)
		}
	})
}
