package migrator

import (
	"strings"
	"testing"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

func TestValidationObjectActionUsesSkippedForNonModules(t *testing.T) {
	if got := validationObjectAction("tables"); got != contracts.ActionValidateSkipped {
		t.Fatalf("expected validate_skipped for tables, got %q", got)
	}
	if got := validationObjectAction("views"); got != contracts.ActionValidateChecked {
		t.Fatalf("expected validate_checked for views, got %q", got)
	}
}

func TestValidationMarkSuccessesDoesNotWriteAttempts(t *testing.T) {
	execer := &stubExecer{result: stubResult{rows: 2}}
	recorder := validationRecorder{writer: newMetadataWriter(config.Config{}, execer, "run-1")}
	objects := []parser.Object{
		{NormalizedKey: "reporting/views/monthly", Kind: "views"},
		{NormalizedKey: "reporting/tables/snapshot", Kind: "tables"},
	}

	if err := recorder.markSuccesses(t.Context(), objects); err != nil {
		t.Fatalf("unexpected validation success error: %v", err)
	}
	if len(execer.calls) != 1 {
		t.Fatalf("expected one batched update, got %#v", execer.calls)
	}
	if strings.Contains(execer.calls[0].query, "INSERT INTO __migrator.attempts") {
		t.Fatalf("expected no attempt inserts, got %#v", execer.calls)
	}
	if !strings.Contains(execer.calls[0].query, "UPDATE i") || !strings.Contains(execer.calls[0].query, "VALUES") {
		t.Fatalf("expected batched item update query, got %#v", execer.calls)
	}
}
