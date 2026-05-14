package types

import "time"

const SQLServerMaxParameters = 2100

const (
	DBAuthSQL        = "sql"
	DBAuthIntegrated = "integrated"

	UpdatePolicyNone         = "none"
	UpdatePolicyModulesOnly  = "modules_only"
	UpdatePolicyAllSupported = "all_supported"

	TransactionModeScript = "script"
	TransactionModeNone   = "none"
)

type Config struct {
	Env, SQLRoot, SQLBase, ReportDir, LogLevel string
	JSONLogs, SkipValidate, Confirm, PlanJSON  bool
	CommandTimeout, ScriptTimeout, LockTimeout time.Duration
	PlanFile, RepairTarget                     string

	Server, Port, Database, DBAuth, User, Password string
	Encrypt, TrustServerCertificate                bool

	GitCommit, GitBranch, PipelineRunID, PipelineURL, Actor string
	ToolVersion, ToolCommit                                 string

	UpdatePolicy    string
	TransactionMode string
}

func (c Config) DBAuthMode() string {
	if c.DBAuth == "" {
		return DBAuthSQL
	}
	return c.DBAuth
}

func (c Config) EffectiveUpdatePolicy() string {
	if c.UpdatePolicy == "" {
		return UpdatePolicyAllSupported
	}
	return c.UpdatePolicy
}

func (c Config) EffectiveTransactionMode() string {
	if c.TransactionMode == "" {
		return TransactionModeScript
	}
	return c.TransactionMode
}
