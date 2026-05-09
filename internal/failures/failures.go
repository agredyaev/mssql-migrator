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
		failure := contracts.Failure{Phase: strings.TrimSpace(phase), SQLRoot: strings.TrimSpace(cfg.SQLRoot), Base: strings.TrimSpace(cfg.SQLBase)}
		failure.Error = Envelope(failure)
		return failure
	}
	var wrapped causeCarrier
	if errors.As(err, &wrapped) {
		return BuildWithCause(cfg, phase, wrapped.FailureBase(), wrapped.FailureCause())
	}
	return BuildWithCause(cfg, phase, err, nil)
}

func BuildWithCause(cfg config.Config, phase string, base error, cause error) contracts.Failure {
	path, class, reason, sql := describe(base, cause)
	failure := contracts.Failure{
		Script:  strings.TrimSpace(path),
		Phase:   strings.TrimSpace(phase),
		SQLRoot: strings.TrimSpace(cfg.SQLRoot),
		Base:    strings.TrimSpace(cfg.SQLBase),
		Class:   strings.TrimSpace(class),
		Reason:  strings.TrimSpace(reason),
		SQL:     strings.TrimSpace(sql),
	}
	failure.Error = Envelope(failure)
	return failure
}

func BuildPlanBlocked(cfg config.Config, plan contracts.MigrationPlan) contracts.Failure {
	reason := "plan is blocked"
	if len(plan.BlockReasons) > 0 {
		reason = logger.Redact(strings.TrimSpace(plan.BlockReasons[0]))
	} else if len(plan.Failures) > 0 {
		reason = logger.Redact(strings.TrimSpace(plan.Failures[0]))
	}
	failure := contracts.Failure{
		Script:  extractPathFromMessage(reason),
		Phase:   "plan_failed",
		SQLRoot: strings.TrimSpace(cfg.SQLRoot),
		Base:    strings.TrimSpace(cfg.SQLBase),
		Class:   Classify(fmt.Errorf("%s", reason), nil),
		Reason:  reason,
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
	if detail := firstDetail(cause, base); detail != nil {
		if class := strings.TrimSpace(detail.FailureClass()); class != "" {
			return logger.Redact(class)
		}
	}
	message := strings.ToLower(messageFor(base, cause))
	switch {
	case strings.Contains(message, "approved plan missing"):
		return "approved plan missing"
	case strings.Contains(message, "approved plan mismatch"):
		return "approved plan mismatch"
	case strings.Contains(message, "existing object changed") || strings.Contains(message, "metadata drift") || errors.Is(base, contracts.ErrChecksumMismatch):
		return "incompatible existing object"
	case strings.Contains(message, "missing_metadata_ddl_permission") || strings.Contains(message, "missing metadata ddl permission"):
		return "missing metadata DDL permission"
	case strings.Contains(message, "metadata_schema_incompatible") || strings.Contains(message, "metadata schema incompatible"):
		return "metadata schema incompatible"
	case strings.Contains(message, "missing schema creation permission"):
		return "missing schema creation permission"
	case strings.Contains(message, "missing object ddl permission"):
		return "missing object DDL permission"
	case strings.Contains(message, "missing parent object"):
		return "missing parent object"
	case strings.Contains(message, "missing catalog visibility"):
		return "missing catalog visibility"
	case strings.Contains(message, "invalid repository layout"):
		return "invalid repository layout"
	case strings.Contains(message, "invalid_or_missing_sql_scripts_root"):
		return "invalid or missing SQL scripts root"
	case strings.Contains(message, "invalid_or_missing_base_selection"):
		return "invalid or missing base selection"
	case strings.Contains(message, "invalid comparison mode"):
		return "invalid comparison mode"
	case strings.Contains(message, "invalid_update_policy"):
		return "invalid update policy"
	case strings.Contains(message, "invalid_transaction_mode"):
		return "invalid transaction mode"
	case errors.Is(base, contracts.ErrConnection):
		return "connection failed"
	case errors.Is(base, contracts.ErrLockTimeout):
		return "lock timeout"
	case errors.Is(base, contracts.ErrValidation):
		return "validation failure"
	case errors.Is(base, contracts.ErrSQLExecution):
		return "sql execution failure"
	case errors.Is(base, contracts.ErrCriticalState):
		return "critical metadata state"
	case strings.Contains(message, "missing required config") || strings.Contains(message, "unknown command") || strings.Contains(message, "confirm flag required") || errors.Is(base, contracts.ErrInvalidInput):
		return "invalid input"
	default:
		return "invalid input"
	}
}

func describe(base error, cause error) (string, string, string, string) {
	detail := firstDetail(cause, base)
	path := ""
	class := ""
	reason := ""
	sql := ""
	if detail != nil {
		path = logger.Redact(strings.TrimSpace(detail.FailurePath()))
		class = logger.Redact(strings.TrimSpace(detail.FailureClass()))
		reason = logger.Redact(strings.TrimSpace(detail.FailureReason()))
		sql = logger.Redact(strings.TrimSpace(detail.FailureSQL()))
	}
	if class == "" {
		class = Classify(base, cause)
	}
	if reason == "" {
		reason = reasonFor(base, cause)
	}
	if sql == "" {
		sql = sqlFor(base, cause, class)
	}
	if path == "" {
		path = extractPathFromMessage(reason)
	}
	return path, class, reason, sql
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
		{prefix: "existing object changed: ", suffix: ""},
		{prefix: "invalid object state: ", suffix: ""},
		{prefix: "repair target is missing from the database: ", suffix: ""},
		{prefix: "repair target has no successful metadata row: ", suffix: ""},
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
