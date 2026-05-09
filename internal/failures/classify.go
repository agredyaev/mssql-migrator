package failures

import (
	"errors"
	"strings"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
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
		if value := logger.Redact(strings.TrimSpace(detail.FailurePath())); value != "" {
			path = value
		}
		if value := logger.Redact(strings.TrimSpace(detail.FailureReason())); value != "" {
			reason = value
		}
		if value := logger.Redact(strings.TrimSpace(detail.FailureSQL())); value != "" {
			return Classification{Class: class, Path: path, Reason: reason, SQL: value}
		}
	}
	return Classification{Class: class, Path: path, Reason: reason, SQL: sqlFor(base, cause, class)}
}

func classify(base error, cause error) string {
	if detail := firstDetail(cause, base); detail != nil {
		if class := strings.TrimSpace(detail.FailureClass()); class != "" {
			return logger.Redact(class)
		}
	}
	message := strings.ToLower(messageFor(base, cause))
	switch {
	case errors.Is(base, contracts.ErrApprovedPlanMissing) || errors.Is(cause, contracts.ErrApprovedPlanMissing):
		return contracts.ErrApprovedPlanMissing.Error()
	case errors.Is(base, contracts.ErrApprovedPlanMismatch) || errors.Is(cause, contracts.ErrApprovedPlanMismatch):
		return contracts.ErrApprovedPlanMismatch.Error()
	case strings.Contains(message, contracts.ErrApprovedPlanMissing.Error()):
		return contracts.ErrApprovedPlanMissing.Error()
	case strings.Contains(message, contracts.ErrApprovedPlanMismatch.Error()):
		return contracts.ErrApprovedPlanMismatch.Error()
	case strings.Contains(message, "existing object changed") || errors.Is(base, contracts.ErrMetadataDrift) || errors.Is(cause, contracts.ErrMetadataDrift) || errors.Is(base, contracts.ErrChecksumMismatch):
		return "incompatible existing object"
	case errors.Is(base, metadata.ErrMissingDDLPermission) || errors.Is(cause, metadata.ErrMissingDDLPermission) || strings.Contains(message, "missing_metadata_ddl_permission") || strings.Contains(message, "missing metadata ddl permission"):
		return "missing metadata DDL permission"
	case errors.Is(base, metadata.ErrSchemaIncompatible) || errors.Is(cause, metadata.ErrSchemaIncompatible) || strings.Contains(message, "metadata_schema_incompatible") || strings.Contains(message, "metadata schema incompatible"):
		return "metadata schema incompatible"
	case errors.Is(base, contracts.ErrMissingSchemaPermission) || errors.Is(cause, contracts.ErrMissingSchemaPermission) || strings.Contains(message, contracts.ErrMissingSchemaPermission.Error()):
		return contracts.ErrMissingSchemaPermission.Error()
	case errors.Is(base, contracts.ErrMissingObjectPermission) || errors.Is(cause, contracts.ErrMissingObjectPermission) || strings.Contains(message, strings.ToLower(contracts.ErrMissingObjectPermission.Error())):
		return contracts.ErrMissingObjectPermission.Error()
	case errors.Is(base, contracts.ErrMissingParentObject) || errors.Is(cause, contracts.ErrMissingParentObject) || strings.Contains(message, contracts.ErrMissingParentObject.Error()):
		return contracts.ErrMissingParentObject.Error()
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
