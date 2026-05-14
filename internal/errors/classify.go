package errors

import (
	"errors"
	"strings"
)

type Classification struct {
	Class  string
	Path   string
	Reason string
	SQL    string
}

func ClassifyDetails(base error, cause error) Classification {
	class := classify(base, cause)
	reason := reasonFor(base, cause)
	path := extractPathFromMessage(reason)

	if detail := firstDetail(cause, base); detail != nil {
		if v := strings.TrimSpace(detail.FailurePath()); v != "" {
			path = v
		}
		if v := strings.TrimSpace(detail.FailureReason()); v != "" {
			reason = v
		}
		if v := strings.TrimSpace(detail.FailureSQL()); v != "" {
			return Classification{Class: class, Path: path, Reason: reason, SQL: v}
		}
	}
	return Classification{Class: class, Path: path, Reason: reason, SQL: sqlFor(base, cause, class)}
}

func classify(base error, cause error) string {
	if detail := firstDetail(cause, base); detail != nil {
		if class := strings.TrimSpace(detail.FailureClass()); class != "" {
			return class
		}
	}
	message := strings.ToLower(messageFor(base, cause))
	switch {
	case errors.Is(base, ErrApprovedPlanMissing) || errors.Is(cause, ErrApprovedPlanMissing):
		return ErrApprovedPlanMissing.Error()
	case errors.Is(base, ErrApprovedPlanMismatch) || errors.Is(cause, ErrApprovedPlanMismatch):
		return ErrApprovedPlanMismatch.Error()
	case strings.Contains(message, "existing object changed") || errors.Is(base, ErrMetadataDrift) || errors.Is(cause, ErrMetadataDrift) || errors.Is(base, ErrChecksumMismatch):
		return "incompatible existing object"
	case errors.Is(base, ErrMissingDDLPermission) || errors.Is(cause, ErrMissingDDLPermission) || strings.Contains(message, "missing_metadata_ddl_permission") || strings.Contains(message, "missing metadata ddl permission"):
		return "missing metadata DDL permission"
	case errors.Is(base, ErrSchemaIncompatible) || errors.Is(cause, ErrSchemaIncompatible) || strings.Contains(message, "metadata_schema_incompatible") || strings.Contains(message, "metadata schema incompatible"):
		return "metadata schema incompatible"
	case errors.Is(base, ErrMissingSchemaPermission) || errors.Is(cause, ErrMissingSchemaPermission) || strings.Contains(message, ErrMissingSchemaPermission.Error()):
		return ErrMissingSchemaPermission.Error()
	case errors.Is(base, ErrMissingObjectPermission) || errors.Is(cause, ErrMissingObjectPermission) || strings.Contains(message, strings.ToLower(ErrMissingObjectPermission.Error())):
		return ErrMissingObjectPermission.Error()
	case errors.Is(base, ErrMissingParentObject) || errors.Is(cause, ErrMissingParentObject) || strings.Contains(message, ErrMissingParentObject.Error()):
		return ErrMissingParentObject.Error()
	case strings.Contains(message, "missing catalog visibility"):
		return "missing catalog visibility"
	case strings.Contains(message, "invalid repository layout"):
		return "invalid repository layout"
	case strings.Contains(message, "invalid_or_missing_sql_scripts_root"):
		return "invalid or missing SQL scripts root"
	case errors.Is(base, ErrConfig):
		return ErrConfig.Error()
	case errors.Is(cause, ErrConfig):
		return ErrConfig.Error()
	case errors.Is(base, ErrConnection):
		return "connection failed"
	case errors.Is(cause, ErrConnection):
		return "connection failed"
	case errors.Is(base, ErrLockTimeout):
		return "lock timeout"
	case errors.Is(cause, ErrLockTimeout):
		return "lock timeout"
	case errors.Is(base, ErrValidation):
		return "validation failure"
	case errors.Is(cause, ErrValidation):
		return "validation failure"
	case errors.Is(base, ErrSQLExecution):
		return "sql execution failure"
	case errors.Is(cause, ErrSQLExecution):
		return "sql execution failure"
	case errors.Is(base, ErrCriticalState):
		return "critical metadata state"
	case errors.Is(cause, ErrCriticalState):
		return "critical metadata state"
	case strings.Contains(message, "unknown command") || strings.Contains(message, "confirm flag required") || errors.Is(base, ErrInvalidInput):
		return "invalid input"
	case errors.Is(cause, ErrInvalidInput):
		return "invalid input"
	case cause != nil:
		return "runtime failure"
	case base != nil:
		return "runtime failure"
	default:
		return "runtime failure"
	}
}

func reasonFor(base error, cause error) string {
	if cause != nil {
		return strings.TrimSpace(cause.Error())
	}
	if base == nil {
		return "-"
	}
	return strings.TrimSpace(base.Error())
}

func sqlFor(base error, cause error, class string) string {
	if !shouldIncludeSQL(class) {
		return ""
	}
	if cause != nil {
		return strings.TrimSpace(cause.Error())
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
		return cause.Error()
	}
	message := base.Error()
	if cause != nil {
		message += ": " + cause.Error()
	}
	return message
}

func extractPathFromMessage(message string) string {
	lowerMessage := strings.ToLower(strings.TrimSpace(message))

	for _, pattern := range pathPatterns {
		prefix := strings.ToLower(pattern.prefix)
		index := strings.Index(lowerMessage, prefix)
		if index < 0 {
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

type pathPatternEntry struct {
	prefix string
	suffix string
}

var pathPatterns = []pathPatternEntry{
	{prefix: "blocked existing object update: ", suffix: " must start with create or alter"},
	{prefix: "blocked existing object change: ", suffix: " is already tracked"},
	{prefix: "existing object changed: "},
	{prefix: "invalid object state: "},
	{prefix: "repair-checksum is not needed for ", suffix: ":"},
	{prefix: "repair-checksum cannot run for ", suffix: ":"},
	{prefix: "repair target is missing from the database: "},
	{prefix: "repair target has no successful metadata row: "},
	{prefix: "repair-checksum target not found in repo layout: "},
	{prefix: "repair-checksum target not found in current plan: "},
	{prefix: "repair target not found in repo layout: "},
	{prefix: "missing parent object for ", suffix: ":"},
	{prefix: "missing object ddl permission for ", suffix: ":"},
	{prefix: "missing schema creation permission for ", suffix: ":"},
	{prefix: "create object ", suffix: ":"},
	{prefix: "create schema ", suffix: ":"},
	{prefix: "check ", suffix: " failed:"},
	{prefix: "missing schema: "},
	{prefix: "missing managed object: "},
}

type detailCarrier interface {
	FailurePath() string
	FailureClass() string
	FailureReason() string
	FailureSQL() string
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

func Classify(base error, cause error) string {
	return classify(base, cause)
}
