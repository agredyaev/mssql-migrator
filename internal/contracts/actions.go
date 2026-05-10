package contracts

import (
	"reporting-db-migrations/internal/commands"
)

const (
	SchemaActionExists       = "exists"
	SchemaActionCreateSchema = "create_schema"

	CommandPlan           = commands.Plan
	CommandMigrate        = commands.Migrate
	CommandValidate       = commands.Validate
	CommandBaseline       = commands.Baseline
	CommandRepairChecksum = commands.RepairChecksum

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
	transactionModeNone = "none"
)

func TransactionModeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction {
		return transactionModeNone
	}
	return defaultMode
}

func NoTransactionForObject(defaultMode string, noTransaction bool) bool {
	return noTransaction || defaultMode == transactionModeNone
}

func RollbackScope(defaultMode string) string {
	if defaultMode == transactionModeNone {
		return RollbackScopeNone
	}
	return RollbackScopeScript
}

func RollbackScopeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction || defaultMode == transactionModeNone {
		return RollbackScopeNone
	}
	return RollbackScopeScript
}
