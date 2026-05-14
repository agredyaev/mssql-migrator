package types

import "strings"

func NormalizedKey(schema, kind, name string) string {
	return strings.ToLower(schema + "/" + kind + "/" + name)
}
