package driver_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/testutil"
)

// legacyConnStableKey matches the pre-unification logic in db/audit packages.
func legacyConnStableKey(conn driver.Conn) string {
	v := reflect.ValueOf(conn)
	if !v.IsValid() {
		return "<nil>"
	}
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		return fmt.Sprintf("%T:%x", conn, v.Pointer())
	}
	return fmt.Sprintf("%T", conn)
}

func TestConnStableKey_matchesLegacy(t *testing.T) {
	var nilConn driver.Conn
	ptr := &testutil.MockConn{}
	var nilPtr *testutil.MockConn

	for _, tc := range []struct {
		name string
		conn driver.Conn
	}{
		{"nil_interface", nilConn},
		{"mock_non_nil_pointer", ptr},
		{"typed_nil_pointer", nilPtr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := driver.ConnStableKey(tc.conn)
			want := legacyConnStableKey(tc.conn)
			if got != want {
				t.Fatalf("ConnStableKey = %q, legacy = %q", got, want)
			}
		})
	}
}

func TestConnTypeNameLower_matchesFmtType(t *testing.T) {
	var nilConn driver.Conn
	ptr := &testutil.MockConn{}
	var nilPtr *testutil.MockConn
	for _, conn := range []driver.Conn{nilConn, ptr, nilPtr} {
		want := strings.ToLower(fmt.Sprintf("%T", conn))
		if got := driver.ConnTypeNameLower(conn); got != want {
			t.Fatalf("ConnTypeNameLower(%v) = %q, want %q", conn, got, want)
		}
	}
}

// stubValueConn implements Conn as a non-pointer value in an interface (rare).
type stubValueConn struct{}

func (stubValueConn) QueryContext(context.Context, string, ...any) (driver.Rows, error) {
	return nil, nil
}
func (stubValueConn) QueryStringsContext(context.Context, string, []string) (driver.Rows, error) {
	return nil, nil
}
func (stubValueConn) QueryStringSlicesContext(context.Context, string, []string, []string) (driver.Rows, error) {
	return nil, nil
}
func (stubValueConn) ExecContext(context.Context, string, ...any) (driver.Result, error) { return nil, nil }
func (stubValueConn) Ping(context.Context) error                                            { return nil }
func (stubValueConn) Close() error                                                          { return nil }

func TestConnStableKey_nonPointerConcrete(t *testing.T) {
	var c driver.Conn = stubValueConn{}
	got := driver.ConnStableKey(c)
	want := legacyConnStableKey(c)
	if got != want {
		t.Fatalf("ConnStableKey = %q, legacy = %q", got, want)
	}
}
