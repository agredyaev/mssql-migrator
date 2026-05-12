package migrator

import "testing"

func TestExecutionPlanOptionsCanSkipTransitionPreflight(t *testing.T) {
	options := executionPlanOptions{skipTransitionPreflight: true}
	if !options.skipTransitionPreflight {
		t.Fatalf("expected approved-plan fast path to set skipTransitionPreflight: %#v", options)
	}
}
