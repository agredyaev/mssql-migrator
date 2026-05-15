package errors

import (
	"errors"

	"reporting-db-migrations/internal/types"
)

type Outcome struct {
	Failure  types.Failure
	ExitCode int
}

func Evaluate(cfg types.Config, phase string, err error) Outcome {
	if err == nil {
		return Outcome{ExitCode: types.ExitOK}
	}
	return Outcome{Failure: Build(cfg, phase, err), ExitCode: ExitCode(err)}
}

func EvaluateWithCause(cfg types.Config, phase string, base error, cause error) Outcome {
	err := Wrap(base, cause)
	if err == nil {
		return Outcome{ExitCode: types.ExitOK}
	}
	return Outcome{Failure: BuildWithCause(cfg, phase, base, cause), ExitCode: ExitCode(err)}
}

func EvaluatePlanBlocked(cfg types.Config, plan types.MigrationPlan) Outcome {
	return Outcome{Failure: BuildPlanBlocked(cfg, plan), ExitCode: types.ExitPlanBlocked}
}

func ExitCode(err error) int {
	switch {
	case err == nil:
		return types.ExitOK
	case errors.Is(err, ErrPlanBlocked):
		return types.ExitPlanBlocked
	case errors.Is(err, ErrConfig):
		return types.ExitConfigError
	case errors.Is(err, ErrConnection):
		return types.ExitConnError
	case errors.Is(err, ErrApprovedPlanMissing):
		return types.ExitInvalidInput
	case errors.Is(err, ErrApprovedPlanMismatch):
		return types.ExitInvalidInput
	case errors.Is(err, ErrMetadataDrift):
		return types.ExitChecksumMismatch
	case errors.Is(err, ErrChecksumMismatch):
		return types.ExitChecksumMismatch
	case errors.Is(err, ErrSQLExecution):
		return types.ExitSQLExecution
	case errors.Is(err, ErrValidation):
		return types.ExitValidation
	case errors.Is(err, ErrLockTimeout), errors.Is(err, ErrLockFailed):
		return types.ExitLockTimeout
	case errors.Is(err, ErrInvalidInput):
		return types.ExitInvalidInput
	case errors.Is(err, ErrCriticalState):
		return types.ExitCriticalState
	case errors.Is(err, ErrMissingSchemaPermission),
		errors.Is(err, ErrMissingObjectPermission),
		errors.Is(err, ErrMissingParentObject),
		errors.Is(err, ErrMissingDDLPermission),
		errors.Is(err, ErrSchemaIncompatible):
		return types.ExitInvalidInput
	default:
		return types.ExitGeneralError
	}
}
