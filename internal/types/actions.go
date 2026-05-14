package types

const (
	SchemaActionExists       = "exists"
	SchemaActionCreateSchema = "create_schema"

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

	ScriptTypeSchema   = "schema"
	ScriptTypeObject   = "object"
	ScriptTypeValidate = "validate"
	ScriptTypeBaseline = "baseline"
	ScriptTypeRepair   = "repair"

	RollbackScopeScript = "script"
	RollbackScopeNone   = "none"
)
