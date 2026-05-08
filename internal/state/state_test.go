package state

import "testing"

func TestNewUsesLatestSuccessfulAttemptPerScript(t *testing.T) {
	state := New([]Attempt{{ScriptName: "R001__views.sql", Checksum: "old", Success: true}, {ScriptName: "R001__views.sql", Checksum: "new", Success: true}, {ScriptName: "R002__bad.sql", Checksum: "bad", Success: false}})
	latest := state.SuccessByScript["R001__views.sql"]
	if latest.Checksum != "new" {
		t.Fatalf("expected latest checksum, got %s", latest.Checksum)
	}
	if len(state.Failures) != 1 {
		t.Fatalf("expected one failure, got %d", len(state.Failures))
	}
}
