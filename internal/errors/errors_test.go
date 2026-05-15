package errors

import (
	"errors"
	"testing"

	"reporting-db-migrations/internal/types"
)

func TestWrap(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		if err := Wrap(nil, nil); err != nil {
			t.Errorf("Wrap(nil, nil) = %v, want nil", err)
		}
	})
	t.Run("base nil", func(t *testing.T) {
		cause := errors.New("cause")
		if err := Wrap(nil, cause); err != cause {
			t.Errorf("Wrap(nil, cause) = %v, want cause", err)
		}
	})
	t.Run("cause nil", func(t *testing.T) {
		base := errors.New("base")
		if err := Wrap(base, nil); err != base {
			t.Errorf("Wrap(base, nil) = %v, want base", err)
		}
	})
	t.Run("both present", func(t *testing.T) {
		base := ErrSQLExecution
		cause := errors.New("mssql: invalid column name")
		err := Wrap(base, cause)
		if !errors.Is(err, base) {
			t.Error("Wrap: errors.Is(base) failed")
		}
		if !errors.Is(err, cause) {
			t.Error("Wrap: errors.Is(cause) failed")
		}
	})
}

func TestErrorWithCause_Unwrap(t *testing.T) {
	base := ErrConnection
	cause := errors.New("connection refused")
	err := Wrap(base, cause)

	unwrapped := err.(interface{ Unwrap() []error }).Unwrap()
	if len(unwrapped) != 2 {
		t.Fatalf("Unwrap() returned %d errors, want 2", len(unwrapped))
	}
}

func TestClassifyConfigError(t *testing.T) {
	class := Classify(ErrConfig, nil)
	if class != "configuration error" {
		t.Errorf("Classify(ErrConfig) = %q, want %q", class, "configuration error")
	}
}

func TestClassifyConnectionError(t *testing.T) {
	class := Classify(ErrConnection, nil)
	if class != "connection failed" {
		t.Errorf("class = %q", class)
	}
}

func TestClassifyNilError(t *testing.T) {
	class := Classify(nil, nil)
	if class != "runtime failure" {
		t.Errorf("class = %q", class)
	}
}

func TestClassifyDetails(t *testing.T) {
	details := ClassifyDetails(ErrSQLExecution, errors.New("mssql: syntax error at GO"))
	if details.Class != "sql execution failure" {
		t.Errorf("Class = %q", details.Class)
	}
	if details.Reason != "mssql: syntax error at GO" {
		t.Errorf("Reason = %q", details.Reason)
	}
}

func TestClassifyPriority(t *testing.T) {
	t.Run("approved plan missing wins over config", func(t *testing.T) {
		class := Classify(ErrApprovedPlanMissing, ErrConfig)
		if class != ErrApprovedPlanMissing.Error() {
			t.Errorf("class = %q", class)
		}
	})
}

func TestBuildFailure(t *testing.T) {
	cfg := types.Config{SQLRoot: "/sql", SQLBase: "dwh"}
	failure := Build(cfg, "plan", ErrInvalidInput)
	if failure.Phase != "plan" {
		t.Errorf("Phase = %q", failure.Phase)
	}
	if failure.Error == "" {
		t.Error("Error envelope is empty")
	}
}

func TestEnvelope(t *testing.T) {
	f := types.Failure{
		Phase: "plan", SQLRoot: "/sql", Base: "dwh",
		Class: "configuration error", Reason: "missing required config",
	}
	env := Envelope(f)
	if env == "" {
		t.Error("Envelope is empty")
	}
}

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		err      error
		wantCode int
	}{
		{nil, types.ExitOK},
		{ErrConfig, types.ExitConfigError},
		{ErrConnection, types.ExitConnError},
		{ErrChecksumMismatch, types.ExitChecksumMismatch},
		{ErrSQLExecution, types.ExitSQLExecution},
		{ErrValidation, types.ExitValidation},
		{ErrLockTimeout, types.ExitLockTimeout},
		{ErrInvalidInput, types.ExitInvalidInput},
		{ErrCriticalState, types.ExitCriticalState},
		{ErrApprovedPlanMissing, types.ExitInvalidInput},
		{ErrApprovedPlanMismatch, types.ExitInvalidInput},
		{ErrMetadataDrift, types.ExitChecksumMismatch},
		{errors.New("unknown"), types.ExitGeneralError},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			code := ExitCode(tt.err)
			if code != tt.wantCode {
				t.Errorf("ExitCode(%v) = %d, want %d", tt.err, code, tt.wantCode)
			}
		})
	}
}

func TestExitCode_Wrapped(t *testing.T) {
	err := Wrap(ErrSQLExecution, errors.New("syntax error"))
	code := ExitCode(err)
	if code != types.ExitSQLExecution {
		t.Errorf("ExitCode(wrapped) = %d, want %d", code, types.ExitSQLExecution)
	}
}

func TestEvaluateOK(t *testing.T) {
	cfg := types.Config{}
	outcome := Evaluate(cfg, "plan", nil)
	if outcome.ExitCode != types.ExitOK {
		t.Errorf("ExitCode = %d, want 0", outcome.ExitCode)
	}
}

func TestEvaluateError(t *testing.T) {
	cfg := types.Config{}
	outcome := Evaluate(cfg, "plan", ErrConfig)
	if outcome.ExitCode != types.ExitConfigError {
		t.Errorf("ExitCode = %d", outcome.ExitCode)
	}
	if outcome.Failure.Class == "" {
		t.Error("Failure.Class is empty")
	}
}

func TestEvaluatePlanBlocked(t *testing.T) {
	cfg := types.Config{}
	plan := types.MigrationPlan{
		Blocked:      true,
		BlockReasons: []string{"table foo changed but no transition file"},
	}
	outcome := EvaluatePlanBlocked(cfg, plan)
	if outcome.ExitCode != types.ExitPlanBlocked {
		t.Errorf("ExitCode = %d", outcome.ExitCode)
	}
}
