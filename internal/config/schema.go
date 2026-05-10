package config

import "flag"

type FieldSpec struct {
	Flag  string
	Env   string
	Usage string
	Bind  func(*flag.FlagSet, *Input)
}

var inputFieldSpecs = []FieldSpec{
	stringField("env", "RM_ENV", "", "target environment", func(input *Input) *string { return &input.Env }),
	stringField("sql-root", "RM_SQL_ROOT", "", "SQL scripts root directory", func(input *Input) *string { return &input.SQLRoot }),
	stringField("sql-base", "RM_SQL_BASE", "", "SQL base directory under the root", func(input *Input) *string { return &input.SQLBase }),
	stringField("report-dir", "RM_REPORT_DIR", "./reports", "report output directory", func(input *Input) *string { return &input.ReportDir }),
	stringField("log-level", "RM_LOG_LEVEL", "info", "log level", func(input *Input) *string { return &input.LogLevel }),
	boolField("json-logs", "RM_JSON_LOGS", false, "emit JSON logs", func(input *Input) *bool { return &input.JSONLogs }),
	boolField("json", "RM_PLAN_JSON", false, "emit plan JSON to stdout", func(input *Input) *bool { return &input.PlanJSON }),
	stringField("timeout", "RM_TIMEOUT", "900s", "command timeout", func(input *Input) *string { return &input.CommandTimeout }),
	stringField("script-timeout", "RM_SCRIPT_TIMEOUT", "600s", "per-script timeout", func(input *Input) *string { return &input.ScriptTimeout }),
	stringField("lock-timeout", "RM_LOCK_TIMEOUT", "60s", "SQL app lock timeout", func(input *Input) *string { return &input.LockTimeout }),
	stringField("env-file", "RM_ENV_FILE", "", "optional env file with RM_* values", func(input *Input) *string { return &input.EnvFile }),
	stringField("plan-file", "RM_PLAN_FILE", "", "approved migration plan file", func(input *Input) *string { return &input.PlanFile }),
	stringField("script", "RM_REPAIR_SCRIPT", "", "repo object path or normalized key to repair", func(input *Input) *string { return &input.RepairTarget }),
	stringField("update-policy", "RM_UPDATE_POLICY", UpdatePolicyNone, "existing object update policy", func(input *Input) *string { return &input.UpdatePolicy }),
	stringField("transaction-mode", "RM_TRANSACTION_MODE", TransactionModeScript, "transaction mode", func(input *Input) *string { return &input.TransactionMode }),
	boolField("confirm", "RM_CONFIRM", false, "confirm destructive command", func(input *Input) *bool { return &input.Confirm }),
	boolField("skip-validate", "RM_SKIP_VALIDATE", false, "skip validation after migrate", func(input *Input) *bool { return &input.SkipValidate }),
}

func BindFlags(flags *flag.FlagSet, input *Input) {
	for _, spec := range inputFieldSpecs {
		spec.Bind(flags, input)
	}
}

func MarkExplicitFlags(flags *flag.FlagSet, input *Input) {
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "timeout":
			input.commandTimeoutFromFlag = true
		case "script-timeout":
			input.scriptTimeoutFromFlag = true
		case "lock-timeout":
			input.lockTimeoutFromFlag = true
		}
	})
}

func AllowedEnvKeys() map[string]struct{} {
	keys := make(map[string]struct{}, len(inputFieldSpecs)+11)
	for _, spec := range inputFieldSpecs {
		if spec.Env == "" {
			continue
		}
		keys[spec.Env] = struct{}{}
	}
	for _, key := range []string{
		"RM_DB_SERVER",
		"RM_DB_PORT",
		"RM_DB_DATABASE",
		"RM_DB_AUTH",
		"RM_DB_USER",
		"RM_DB_PASSWORD",
		"RM_DB_ENCRYPT",
		"RM_DB_TRUST_SERVER_CERTIFICATE",
		"RM_GIT_COMMIT",
		"RM_GIT_BRANCH",
		"RM_PIPELINE_RUN_ID",
		"RM_PIPELINE_URL",
		"RM_ACTOR",
	} {
		keys[key] = struct{}{}
	}
	return keys
}

func stringField(flagName string, envName string, fallback string, usage string, target func(*Input) *string) FieldSpec {
	return FieldSpec{
		Flag:  flagName,
		Env:   envName,
		Usage: usage,
		Bind: func(flags *flag.FlagSet, input *Input) {
			flags.StringVar(target(input), flagName, Getenv(envName, fallback), usage)
		},
	}
}

func boolField(flagName string, envName string, fallback bool, usage string, target func(*Input) *bool) FieldSpec {
	return FieldSpec{
		Flag:  flagName,
		Env:   envName,
		Usage: usage,
		Bind: func(flags *flag.FlagSet, input *Input) {
			flags.BoolVar(target(input), flagName, GetenvBool(envName, fallback), usage)
		},
	}
}
