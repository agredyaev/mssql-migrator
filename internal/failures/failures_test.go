package failures

import (
	"fmt"
	"strings"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
)

func TestEnvelopeUsesRequiredShape(t *testing.T) {
	failure := BuildWithCause(config.Config{SQLRoot: "/sql", SQLBase: "dwh"}, "plan_failed", contracts.ErrInvalidInput, fmt.Errorf("invalid_or_missing_base_selection: missing"))
	if !strings.Contains(failure.Error, "ERROR plan_failed:") {
		t.Fatalf("expected phase envelope, got %q", failure.Error)
	}
	if !strings.Contains(failure.Error, "sql_root=/sql") || !strings.Contains(failure.Error, "base=dwh") {
		t.Fatalf("expected root/base fields, got %q", failure.Error)
	}
	if !strings.Contains(failure.Error, "class=invalid or missing base selection") {
		t.Fatalf("expected classified envelope, got %q", failure.Error)
	}
	if !strings.Contains(failure.Error, "reason=invalid_or_missing_base_selection: missing") {
		t.Fatalf("expected reason field, got %q", failure.Error)
	}
	if !strings.Contains(failure.Error, "; sql=-") {
		t.Fatalf("expected sql placeholder, got %q", failure.Error)
	}
}
