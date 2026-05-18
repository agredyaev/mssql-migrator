package types

import "time"

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
	GitCommit              string
	GitBranch              string
	ReportDir              string
	LogLevel               string
	TransactionMode        string
	UpdatePolicy           string
	DBAuth                 string
	ToolVersion            string
	PlanFile               string
	RepairTarget           string
	Server                 string
	User                   string
	SQLBase                string
	ToolCommit             string
	Port                   string
	Password               string
	Actor                  string
	PipelineURL            string
	SQLRoot                string
	Database               string
	PipelineRunID          string
	LockTimeout            time.Duration
	ScriptTimeout          time.Duration
	CommandTimeout         time.Duration
	TrustServerCertificate bool
	Encrypt                bool
	JSONLogs               bool
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
