package attempts_test

import (
	"strings"
	"testing"

	"reporting-db-migrations/internal/attempts"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
)

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestSchemaAppliesAuditFieldsAndTrimsMessage(t *testing.T) {
	attempt := attempts.Schema("reporting", contracts.SchemaActionCreateSchema, false, " boom \n", config.Config{
		GitCommit:     "abc",
		GitBranch:     "main",
		PipelineRunID: "run-1",
		PipelineURL:   "https://ci.example/run?token=secret",
		Actor:         "tester",
	})
	if attempt.ScriptName != "reporting" || attempt.ScriptType != contracts.ScriptTypeSchema {
		t.Fatalf("unexpected schema attempt identity: %#v", attempt)
	}
	if attempt.Action != contracts.SchemaActionCreateSchema || attempt.Success {
		t.Fatalf("unexpected schema attempt contract: %#v", attempt)
	}
	if attempt.ErrorMessage != "boom" {
		t.Fatalf("expected trimmed schema error message, got %#v", attempt)
	}
	if attempt.GitCommit != "abc" || attempt.GitBranch != "main" || attempt.PipelineRunID != "run-1" || attempt.AppliedBy != "tester" {
		t.Fatalf("expected audit fields, got %#v", attempt)
	}
	if attempt.PipelineURL == "https://ci.example/run?token=secret" {
		t.Fatalf("expected redacted pipeline url, got %#v", attempt)
	}
}

func TestValidationCheckFailureUsesNoTransactionContract(t *testing.T) {
	attempt := attempts.ValidationCheckFailure(" failure ", config.Config{})
	if attempt.ScriptName != "validation/checks" || attempt.ScriptType != contracts.ScriptTypeValidate {
		t.Fatalf("unexpected validation attempt identity: %#v", attempt)
	}
	if attempt.Action != contracts.ActionFail || attempt.Success {
		t.Fatalf("unexpected validation attempt result: %#v", attempt)
	}
	if attempt.ErrorMessage != "failure" {
		t.Fatalf("expected trimmed validation error message, got %#v", attempt)
	}
	if attempt.TransactionMode != config.TransactionModeNone || attempt.TransactionScope != config.TransactionModeNone || attempt.RollbackScope != contracts.RollbackScopeNone || !attempt.NoTransaction {
		t.Fatalf("expected no-transaction validation contract, got %#v", attempt)
	}
}

func TestRepairSuccessUsesRepairContract(t *testing.T) {
	id := int64(7)
	attempt := attempts.RepairSuccess(parser.Object{NormalizedKey: "reporting/views/monthly", Checksum: "sum"}, &id, config.Config{})
	if attempt.ItemID == nil || *attempt.ItemID != id {
		t.Fatalf("expected repair item id, got %#v", attempt)
	}
	if attempt.ScriptType != contracts.ScriptTypeRepair || attempt.Action != contracts.ActionRepairChecksum || !attempt.Success {
		t.Fatalf("unexpected repair attempt: %#v", attempt)
	}
	if attempt.TransactionMode != config.TransactionModeNone || attempt.TransactionScope != config.TransactionModeNone || attempt.RollbackScope != contracts.RollbackScopeNone || !attempt.NoTransaction {
		t.Fatalf("expected no-transaction repair contract, got %#v", attempt)
	}
}

func TestRedactErrorNilSafeAndRedactsSecrets(t *testing.T) {
	if got := attempts.RedactError(nil); got != "" {
		t.Fatalf("expected empty nil error redaction, got %q", got)
	}
	got := attempts.RedactError(assertErr("password=secret"))
	if got == "" {
		t.Fatal("expected redacted error message")
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("expected redacted secret, got %q", got)
	}
}
