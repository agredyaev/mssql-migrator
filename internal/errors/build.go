package errors

import (
	"errors"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/types"
)

type causeCarrier interface {
	FailureBase() error
	FailureCause() error
}

func Build(cfg types.Config, phase string, err error) types.Failure {
	if err == nil {
		return buildPayload(cfg, phase, Classification{})
	}
	var wrapped causeCarrier
	if errors.As(err, &wrapped) {
		return BuildWithCause(cfg, phase, wrapped.FailureBase(), wrapped.FailureCause())
	}
	return BuildWithCause(cfg, phase, err, nil)
}

func BuildWithCause(cfg types.Config, phase string, base error, cause error) types.Failure {
	return buildPayload(cfg, phase, ClassifyDetails(base, cause))
}

func BuildPlanBlocked(cfg types.Config, plan types.MigrationPlan) types.Failure {
	reason := "plan is blocked"
	if len(plan.BlockReasons) > 0 {
		reason = strings.TrimSpace(plan.BlockReasons[0])
	} else if len(plan.Failures) > 0 {
		reason = strings.TrimSpace(plan.Failures[0])
	}
	return buildPayload(cfg, "plan_failed", Classification{
		Path:   extractPathFromMessage(reason),
		Class:  classify(fmt.Errorf("%s", reason), nil),
		Reason: reason,
	})
}

func buildPayload(cfg types.Config, phase string, details Classification) types.Failure {
	failure := types.Failure{
		Script:  strings.TrimSpace(details.Path),
		Phase:   strings.TrimSpace(phase),
		SQLRoot: strings.TrimSpace(cfg.SQLRoot),
		Base:    strings.TrimSpace(cfg.SQLBase),
		Class:   strings.TrimSpace(details.Class),
		Reason:  strings.TrimSpace(details.Reason),
		SQL:     strings.TrimSpace(details.SQL),
	}
	failure.Error = Envelope(failure)
	return failure
}

func Envelope(failure types.Failure) string {
	return fmt.Sprintf(
		"ERROR %s: sql_root=%s base=%s path=%s class=%s reason=%s; sql=%s",
		orDash(failure.Phase),
		orDash(failure.SQLRoot),
		orDash(failure.Base),
		orDash(failure.Script),
		orDash(failure.Class),
		orDash(failure.Reason),
		orDash(failure.SQL),
	)
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
