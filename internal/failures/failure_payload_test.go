package failures

import (
	"strings"
	"testing"

	"reporting-db-migrations/internal/config"
)

func TestBuildFailurePayloadUsesEnvelopeAndConfigFields(t *testing.T) {
	failure := buildFailurePayload(config.Config{SQLRoot: "/sql", SQLBase: "dwh"}, "phase_failed", Classification{
		Path:   "reporting/views/monthly.sql",
		Class:  "invalid input",
		Reason: "boom",
		SQL:    "SELECT 1",
	})
	if failure.Phase != "phase_failed" || failure.SQLRoot != "/sql" || failure.Base != "dwh" {
		t.Fatalf("unexpected failure payload: %#v", failure)
	}
	if !strings.Contains(failure.Error, "ERROR phase_failed:") || !strings.Contains(failure.Error, "path=reporting/views/monthly.sql") {
		t.Fatalf("expected envelope shape, got %q", failure.Error)
	}
}
