package failures

import (
	"errors"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

type detailCarrier interface {
	FailurePath() string
	FailureClass() string
	FailureReason() string
	FailureSQL() string
}

type causeCarrier interface {
	FailureBase() error
	FailureCause() error
}

func Build(cfg config.Config, phase string, err error) contracts.Failure {
	if err == nil {
		return buildFailurePayload(cfg, phase, Classification{})
	}
	var wrapped causeCarrier
	if errors.As(err, &wrapped) {
		return BuildWithCause(cfg, phase, wrapped.FailureBase(), wrapped.FailureCause())
	}
	return BuildWithCause(cfg, phase, err, nil)
}

func BuildWithCause(cfg config.Config, phase string, base error, cause error) contracts.Failure {
	return buildFailurePayload(cfg, phase, ClassifyDetails(base, cause))
}

func BuildPlanBlocked(cfg config.Config, plan contracts.MigrationPlan) contracts.Failure {
	reason := "plan is blocked"
	if len(plan.BlockReasons) > 0 {
		reason = logger.Redact(strings.TrimSpace(plan.BlockReasons[0]))
	} else if len(plan.Failures) > 0 {
		reason = logger.Redact(strings.TrimSpace(plan.Failures[0]))
	}
	return buildFailurePayload(cfg, "plan_failed", Classification{
		Path:   extractPathFromMessage(reason),
		Class:  Classify(fmt.Errorf("%s", reason), nil),
		Reason: reason,
	})
}

func buildFailurePayload(cfg config.Config, phase string, details Classification) contracts.Failure {
	failure := contracts.Failure{
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

func Envelope(failure contracts.Failure) string {
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

func Classify(base error, cause error) string {
	return classify(base, cause)
}

func firstDetail(values ...error) detailCarrier {
	for _, value := range values {
		if value == nil {
			continue
		}
		var detail detailCarrier
		if errors.As(value, &detail) {
			return detail
		}
	}
	return nil
}

func reasonFor(base error, cause error) string {
	if cause != nil {
		return logger.Redact(strings.TrimSpace(cause.Error()))
	}
	if base == nil {
		return "-"
	}
	return logger.Redact(strings.TrimSpace(base.Error()))
}

func sqlFor(base error, cause error, class string) string {
	if !shouldIncludeSQL(class) {
		return ""
	}
	if cause != nil {
		return logger.Redact(strings.TrimSpace(cause.Error()))
	}
	return ""
}

func shouldIncludeSQL(class string) bool {
	switch strings.TrimSpace(class) {
	case "missing metadata DDL permission", "missing schema creation permission", "missing object DDL permission", "missing parent object", "sql execution failure", "validation failure":
		return true
	default:
		return false
	}
}

func messageFor(base error, cause error) string {
	if base == nil && cause == nil {
		return ""
	}
	if base == nil {
		return logger.Redact(cause.Error())
	}
	message := logger.Redact(base.Error())
	if cause != nil {
		message += ": " + logger.Redact(cause.Error())
	}
	return message
}

func extractPathFromMessage(message string) string {
	message = logger.Redact(strings.TrimSpace(message))
	patterns := []struct {
		prefix string
		suffix string
	}{
		{prefix: "blocked existing object update: ", suffix: " must start with create or alter"},
		{prefix: "blocked existing object change: ", suffix: " is already tracked"},
		{prefix: "existing object changed: ", suffix: ""},
		{prefix: "invalid object state: ", suffix: ""},
		{prefix: "repair-checksum is not needed for ", suffix: ":"},
		{prefix: "repair-checksum cannot run for ", suffix: ":"},
		{prefix: "repair target is missing from the database: ", suffix: ""},
		{prefix: "repair target has no successful metadata row: ", suffix: ""},
		{prefix: "repair-checksum target not found in repo layout: ", suffix: ""},
		{prefix: "repair-checksum target not found in current plan: ", suffix: ""},
		{prefix: "repair target not found in repo layout: ", suffix: ""},
		{prefix: "missing parent object for ", suffix: ":"},
		{prefix: "missing object ddl permission for ", suffix: ":"},
		{prefix: "missing schema creation permission for ", suffix: ":"},
		{prefix: "create object ", suffix: ":"},
		{prefix: "create schema ", suffix: ":"},
		{prefix: "check ", suffix: " failed:"},
		{prefix: "missing schema: ", suffix: ""},
		{prefix: "missing managed object: ", suffix: ""},
	}
	lower := strings.ToLower(message)
	for _, pattern := range patterns {
		prefix := strings.ToLower(pattern.prefix)
		index := strings.Index(lower, prefix)
		if index == -1 {
			continue
		}
		start := index + len(prefix)
		value := message[start:]
		if pattern.suffix != "" {
			lowerValue := strings.ToLower(value)
			if end := strings.Index(lowerValue, strings.ToLower(pattern.suffix)); end >= 0 {
				value = value[:end]
			}
		}
		return strings.TrimSpace(value)
	}
	return ""
}

func orDash(value string) string {
	value = logger.Redact(strings.TrimSpace(value))
	if value == "" {
		return "-"
	}
	return value
}
