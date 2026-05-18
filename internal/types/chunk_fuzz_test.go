package types

import (
	"strings"
	"testing"
)

// buildINQueryNaive mirrors the pre-refactor BuildINQuery shape: build one @p
// list string, then strings.Replace the template (including every placeholder
// occurrence). Must stay byte-identical to BuildINQuery for fuzz parity.
func buildINQueryNaive(template, placeholder string, keys []string, startIndex int) (string, []string) {
	n := len(keys)
	if n == 0 {
		return strings.Replace(template, placeholder, "", -1), keys
	}
	if !strings.Contains(template, placeholder) {
		return template, keys
	}
	var b strings.Builder
	appendINPlaceholderList(&b, startIndex, n)
	return strings.Replace(template, placeholder, b.String(), -1), keys
}

// FuzzBuildINQuery_matchesNaiveReplace compares BuildINQuery to the naive
// Replace-based reference (same SQL text and args slice identity).
func FuzzBuildINQuery_matchesNaiveReplace(f *testing.F) {
	f.Add("SELECT * FROM t WHERE c IN ({{list}})", "{{list}}", byte(1))
	f.Add("({{list}}) AND ({{list}})", "{{list}}", byte(1))
	f.Add("no placeholder", "{{list}}", byte(5))
	f.Fuzz(func(t *testing.T, template, placeholder string, startIndex byte) {
		const maxT, maxP = 256, 64
		if len(template) > maxT || len(placeholder) > maxP || placeholder == "" {
			t.Skip()
		}
		keys := []string{"a", "b"}
		start := int(startIndex)%40 + 1
		got, gargs := BuildINQuery(template, placeholder, keys, start)
		want, wargs := buildINQueryNaive(template, placeholder, keys, start)
		if got != want {
			t.Fatalf("BuildINQuery mismatch\ngot:  %q\nwant: %q", got, want)
		}
		if len(gargs) != len(wargs) {
			t.Fatalf("args len got %d want %d", len(gargs), len(wargs))
		}
		for i := range gargs {
			if gargs[i] != wargs[i] {
				t.Fatalf("args[%d] got %q want %q", i, gargs[i], wargs[i])
			}
		}
	})
}

// FuzzBuildDualINQuery_expandsPlaceholders checks placeholder expansion for a
// fixed dual-IN template with bounded random keys (no SQL execution).
func FuzzBuildDualINQuery_expandsPlaceholders(f *testing.F) {
	const tpl = "SELECT 1 WHERE s IN ({{schema_list}}) AND o IN ({{object_list}})"
	f.Add("a", "b")
	f.Fuzz(func(t *testing.T, s1, o1 string) {
		const max = 64
		if len(s1) > max || len(o1) > max {
			t.Skip()
		}
		keys1 := []string{s1}
		keys2 := []string{o1}
		q, args := BuildDualINQuery(tpl, "{{schema_list}}", keys1, "{{object_list}}", keys2, 1)
		if !strings.Contains(q, "@p1") || !strings.Contains(q, "@p2") {
			t.Fatalf("query missing @p placeholders: %q", q)
		}
		if len(args) != 2 || args[0] != s1 || args[1] != o1 {
			t.Fatalf("args = %v, want [%q %q]", args, s1, o1)
		}
	})
}
