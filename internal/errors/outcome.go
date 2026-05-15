package errors

import (
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
	if err == nil {
		return types.ExitOK
	}
	for _, rule := range classifyRules {
		if rule.exitCode == 0 {
			continue
		}
		if matchSentinels(rule.sentinels, err, nil) {
			return rule.exitCode
		}
	}
	return types.ExitGeneralError
}
