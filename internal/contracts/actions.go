package contracts

import "reporting-db-migrations/internal/config"

const (
	SchemaActionExists       = "exists"
	SchemaActionCreateSchema = "create_schema"

	CommandPlan           = "plan"
	CommandMigrate        = "migrate"
	CommandValidate       = "validate"
	CommandBaseline       = "baseline"
	CommandRepairChecksum = "repair-checksum"

	ScriptTypeSchema   = "schema"
	ScriptTypeObject   = "object"
	ScriptTypeValidate = "validate"
	ScriptTypeBaseline = "baseline"
	ScriptTypeRepair   = "repair"

	ActionCreateObject            = "create_object"
	ActionAdoptExisting           = "adopt_existing"
	ActionSkipUnchanged           = "skip_unchanged"
	ActionReprocessChanged        = "reprocess_changed"
	ActionReprocessChangedBlocked = "reprocess_changed_blocked"
	ActionUpdateExistingModule    = "update_existing_module"
	ActionUpdateExistingSupported = "update_existing_supported"
	ActionValidateChecked         = "validate_checked"
	ActionValidateSkipped         = "validate_skipped"
	ActionFail                    = "fail"
	ActionRepairChecksum          = "repair_checksum"

	RollbackScopeScript = "script"
	RollbackScopeNone   = "none"
)

func TransactionModeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction {
		return config.TransactionModeNone
	}
	return defaultMode
}

func RollbackScope(defaultMode string) string {
	if defaultMode == config.TransactionModeNone {
		return RollbackScopeNone
	}
	return RollbackScopeScript
}

func RollbackScopeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction || defaultMode == config.TransactionModeNone {
		return RollbackScopeNone
	}
	return RollbackScopeScript
}
