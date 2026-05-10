package failures

import (
	"encoding/json"
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

func TestBuildFailurePayloadKeepsStructuredFieldsForMachineReaders(t *testing.T) {
	failure := buildFailurePayload(config.Config{SQLRoot: "/sql", SQLBase: "dwh"}, "phase_failed", Classification{
		Path:   "reporting/views/monthly.sql",
		Class:  "runtime failure",
		Reason: "boom; line1\nline2",
		SQL:    "SELECT 1;\nGO",
	})
	payload, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["phase"] != "phase_failed" || decoded["reason"] != "boom; line1\nline2" || decoded["sql"] != "SELECT 1;\nGO" {
		t.Fatalf("expected structured machine-readable fields, got %s", string(payload))
	}
	if decoded["error"] == decoded["reason"] {
		t.Fatalf("expected human envelope alongside structured reason, got %s", string(payload))
	}
}
