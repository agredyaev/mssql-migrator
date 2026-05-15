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

func Classify(base error, cause error) string {
	return classify(base, cause)
}

type classifyRule struct {
	sentinels  []error
	substrings []string
	result     string
}

var classifyRules = []classifyRule{
	{sentinels: []error{ErrApprovedPlanMissing},
		result: ErrApprovedPlanMissing.Error()},
	{sentinels: []error{ErrApprovedPlanMismatch},
		result: ErrApprovedPlanMismatch.Error()},
	{sentinels: []error{ErrChecksumMismatch, ErrMetadataDrift},
		substrings: []string{"existing object changed"},
		result:     "incompatible existing object"},
	{sentinels: []error{ErrMissingDDLPermission},
		substrings: []string{"missing_metadata_ddl_permission", "missing metadata ddl permission"},
		result:     "missing metadata DDL permission"},
	{sentinels: []error{ErrSchemaIncompatible},
		substrings: []string{"metadata_schema_incompatible", "metadata schema incompatible"},
		result:     "metadata schema incompatible"},
	{sentinels: []error{ErrMissingSchemaPermission},
		result: ErrMissingSchemaPermission.Error()},
	{sentinels: []error{ErrMissingObjectPermission},
		result: ErrMissingObjectPermission.Error()},
	{sentinels: []error{ErrMissingParentObject},
		result: ErrMissingParentObject.Error()},
	{substrings: []string{"missing catalog visibility"},
		result: "missing catalog visibility"},
	{substrings: []string{"invalid repository layout"},
		result: "invalid repository layout"},
	{substrings: []string{"invalid_or_missing_sql_scripts_root"},
		result: "invalid or missing SQL scripts root"},
	{sentinels: []error{ErrConfig},
		result: ErrConfig.Error()},
	{sentinels: []error{ErrConnection},
		result: "connection failed"},
	{sentinels: []error{ErrLockFailed},
		result: "lock failed"},
	{sentinels: []error{ErrLockTimeout},
		result: "lock timeout"},
	{sentinels: []error{ErrPlanBlocked},
		result: "plan is blocked"},
	{sentinels: []error{ErrValidation},
		result: "validation failure"},
	{sentinels: []error{ErrSQLExecution},
		result: "sql execution failure"},
	{sentinels: []error{ErrCriticalState},
		result: "critical metadata state"},
	{sentinels: []error{ErrInvalidInput},
		substrings: []string{"unknown command", "confirm flag required"},
		result:     "invalid input"},
}

func classify(base error, cause error) string {
	if detail := firstDetail(cause, base); detail != nil {
		if class := strings.TrimSpace(detail.FailureClass()); class != "" {
			return class
		}
	}
	message := strings.ToLower(messageFor(base, cause))
	for _, rule := range classifyRules {
		if matchSentinels(rule.sentinels, base, cause) || matchSubstrings(rule.substrings, message) {
			return rule.result
		}
	}
	if cause != nil || base != nil {
		return "runtime failure"
	}
	return "runtime failure"
}

func matchSentinels(sentinels []error, base, cause error) bool {
	for _, s := range sentinels {
		if errors.Is(base, s) || errors.Is(cause, s) {
			return true
		}
	}
	return false
}

func matchSubstrings(substrings []string, message string) bool {
	for _, s := range substrings {
		if strings.Contains(message, s) {
			return true
		}
	}
	return false
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
