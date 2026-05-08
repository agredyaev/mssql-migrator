package contracts

import "time"

type ScriptResult struct {
	Script      string `json:"script"`
	Type        string `json:"type"`
	Checksum    string `json:"checksum,omitempty"`
	ExecutionMS int    `json:"execution_ms,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type Failure struct {
	Script string `json:"script,omitempty"`
	Batch  int    `json:"batch,omitempty"`
	Error  string `json:"error"`
}

type ValidationSummary struct {
	ModulesRefreshed int `json:"modules_refreshed"`
	ChecksPassed     int `json:"checks_passed"`
	ChecksFailed     int `json:"checks_failed"`
}

type MigrationPlan struct {
	Tool           string    `json:"tool"`
	ToolVersion    string    `json:"tool_version"`
	ToolCommit     string    `json:"tool_commit"`
	GitCommit      string    `json:"git_commit"`
	GitBranch      string    `json:"git_branch"`
	SQLDirHash     string    `json:"sql_dir_hash"`
	TargetEnv      string    `json:"target_env"`
	TargetDatabase string    `json:"target_database"`
	PlannedAt      time.Time `json:"planned_at"`

	PendingScripts           []ScriptResult `json:"pending_scripts"`
	ChangedRepeatableScripts []ScriptResult `json:"changed_repeatable_scripts"`
	Skipped                  []ScriptResult `json:"skipped"`

	Blocked      bool     `json:"blocked"`
	BlockReasons []string `json:"block_reasons,omitempty"`
}

type MigrationReport struct {
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	ToolCommit  string `json:"tool_commit"`
	Environment string `json:"environment"`
	Database    string `json:"database"`

	GitCommit     string `json:"git_commit"`
	GitBranch     string `json:"git_branch"`
	SQLDirHash    string `json:"sql_dir_hash"`
	PipelineRunID string `json:"pipeline_run_id"`
	PipelineURL   string `json:"pipeline_url"`
	Actor         string `json:"actor"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`

	Result     string            `json:"result"`
	Applied    []ScriptResult    `json:"applied"`
	Skipped    []ScriptResult    `json:"skipped"`
	Failed     *Failure          `json:"failed,omitempty"`
	Validation ValidationSummary `json:"validation"`
}

type ValidationReport struct {
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	ToolCommit  string `json:"tool_commit"`
	Environment string `json:"environment"`
	Database    string `json:"database"`

	GitCommit     string `json:"git_commit"`
	GitBranch     string `json:"git_branch"`
	PipelineRunID string `json:"pipeline_run_id"`
	PipelineURL   string `json:"pipeline_url"`
	Actor         string `json:"actor"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`

	Result     string            `json:"result"`
	Validation ValidationSummary `json:"validation"`
	Failed     *Failure          `json:"failed,omitempty"`
}
