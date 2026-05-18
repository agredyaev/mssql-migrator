package driver

import (
	"fmt"
	"reflect"
	"strings"
)

// ConnStableKey returns a stable string for a connection value for use as a map
// or sync.Map key (inspector cache, audit OpenJSON flags, checksum cache). It
// must stay byte-for-byte compatible with historical call sites that used the
// same reflect + fmt rules.
func ConnStableKey(conn Conn) string {
	v := reflect.ValueOf(conn)
	if !v.IsValid() {
		return "<nil>"
	}
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		return fmt.Sprintf("%T:%x", conn, v.Pointer())
	}
	return fmt.Sprintf("%T", conn)
}

// ConnTypeNameLower is strings.ToLower(fmt.Sprintf("%T", conn)) for cold-path
// heuristics (e.g. substring "mssql" in the concrete driver type name).
func ConnTypeNameLower(conn Conn) string {
	return strings.ToLower(fmt.Sprintf("%T", conn))
}
