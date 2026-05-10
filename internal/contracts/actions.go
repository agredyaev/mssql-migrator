package contracts

import "reporting-db-migrations/internal/commands"

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
)
