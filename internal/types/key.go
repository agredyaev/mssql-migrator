package types

import "strings"

// NormalizedKey builds the canonical lowercase map key "schema/kind/name".
// Do not duplicate this logic with ad-hoc ToLower concatenations at call sites;
// if you optimize this function, keep byte-for-byte parity (see types_test.go).
func NormalizedKey(schema, kind, name string) string {
	var b strings.Builder
	b.Grow(len(schema) + len(kind) + len(name) + 2)
	b.WriteString(schema)
	b.WriteByte('/')
	b.WriteString(kind)
	b.WriteByte('/')
	b.WriteString(name)
	return strings.ToLower(b.String())
}
