package failures

import (
	"errors"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
)

type Outcome struct {
	Failure  contracts.Failure
	ExitCode int
}

func Evaluate(cfg config.Config, phase string, err error) Outcome {
	if err == nil {
		return Outcome{ExitCode: contracts.ExitOK}
	}
	return Outcome{Failure: Build(cfg, phase, err), ExitCode: ExitCode(err)}
}

func EvaluateWithCause(cfg config.Config, phase string, base error, cause error) Outcome {
	err := contracts.Wrap(base, cause)
	if err == nil {
		return Outcome{ExitCode: contracts.ExitOK}
	}
	return Outcome{Failure: BuildWithCause(cfg, phase, base, cause), ExitCode: ExitCode(err)}
}

func EvaluatePlanBlocked(cfg config.Config, plan contracts.MigrationPlan) Outcome {
	return Outcome{Failure: BuildPlanBlocked(cfg, plan), ExitCode: contracts.ExitChecksumMismatch}
}

func ExitCode(err error) int {
	switch {
	case err == nil:
		return contracts.ExitOK
	case errors.Is(err, contracts.ErrConfig):
		return contracts.ExitConfigError
	case errors.Is(err, contracts.ErrConnection):
		return contracts.ExitConnError
	case errors.Is(err, contracts.ErrChecksumMismatch):
		return contracts.ExitChecksumMismatch
	case errors.Is(err, contracts.ErrSQLExecution):
		return contracts.ExitSQLExecution
	case errors.Is(err, contracts.ErrValidation):
		return contracts.ExitValidation
	case errors.Is(err, contracts.ErrLockTimeout):
		return contracts.ExitLockTimeout
	case errors.Is(err, contracts.ErrInvalidInput):
		return contracts.ExitInvalidInput
	case errors.Is(err, contracts.ErrCriticalState):
		return contracts.ExitCriticalState
	default:
		return contracts.ExitGeneralError
	}
}

func Message(base error, cause error) string {
	return messageFor(base, cause)
}
