package bus

import (
	"testing"

	"reporting-db-migrations/internal/types"
)

func TestParseObjectAppliedPayload(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		ev := &types.ObjectEvent{Action: "x"}
		got, ok := ParseObjectAppliedPayload(ev)
		if !ok || len(got) != 1 || got[0] != ev {
			t.Fatalf("got %#v ok=%v", got, ok)
		}
	})
	t.Run("batch", func(t *testing.T) {
		batch := []*types.ObjectEvent{{Action: "a"}, {Action: "b"}}
		got, ok := ParseObjectAppliedPayload(batch)
		if !ok || len(got) != 2 {
			t.Fatalf("got %#v ok=%v", got, ok)
		}
	})
	t.Run("empty batch", func(t *testing.T) {
		got, ok := ParseObjectAppliedPayload([]*types.ObjectEvent{})
		if !ok || got == nil || len(got) != 0 {
			t.Fatalf("got %#v ok=%v", got, ok)
		}
	})
	t.Run("wrong type", func(t *testing.T) {
		got, ok := ParseObjectAppliedPayload("nope")
		if ok || got != nil {
			t.Fatalf("got %#v ok=%v", got, ok)
		}
	})
}

func TestParseObjectFailedPayload(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		ev := &types.FailureEvent{Error: "e"}
		got, ok := ParseObjectFailedPayload(ev)
		if !ok || len(got) != 1 || got[0] != ev {
			t.Fatalf("got %#v ok=%v", got, ok)
		}
	})
	t.Run("batch", func(t *testing.T) {
		batch := []*types.FailureEvent{{Error: "a"}, {Error: "b"}}
		got, ok := ParseObjectFailedPayload(batch)
		if !ok || len(got) != 2 {
			t.Fatalf("got %#v ok=%v", got, ok)
		}
	})
	t.Run("wrong type", func(t *testing.T) {
		got, ok := ParseObjectFailedPayload(42)
		if ok || got != nil {
			t.Fatalf("got %#v ok=%v", got, ok)
		}
	})
}

func TestParseDiffResult(t *testing.T) {
	dr := &types.DiffResult{}
	got, ok := ParseDiffResult(dr)
	if !ok || got != dr {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
	if _, ok := ParseDiffResult("wrong"); ok {
		t.Fatal("expected !ok for wrong payload type")
	}
}

func TestParseValidationResult(t *testing.T) {
	vr := &types.ValidationResult{ModulesRefreshed: 3}
	got, ok := ParseValidationResult(vr)
	if !ok || got != vr {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
	if _, ok := ParseValidationResult("wrong"); ok {
		t.Fatal("expected !ok")
	}
}
