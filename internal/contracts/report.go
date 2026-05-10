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
	Script  string `json:"script,omitempty"`
	Batch   int    `json:"batch,omitempty"`
	Phase   string `json:"phase,omitempty"`
	SQLRoot string `json:"sql_root,omitempty"`
	Base    string `json:"base,omitempty"`
	Class   string `json:"class,omitempty"`
	Reason  string `json:"reason,omitempty"`
	SQL     string `json:"sql,omitempty"`
	// Error keeps the legacy human-readable envelope.
	// Machine consumers should read the sibling structured fields.
	Error string `json:"error"`
}

type ValidationSummary struct {
	ModulesRefreshed int `json:"modules_refreshed"`
	ChecksPassed     int `json:"checks_passed"`
	ChecksFailed     int `json:"checks_failed"`
}

type PlanSummary struct {
	SchemaCount  int `json:"schema_count"`
	ObjectCount  int `json:"object_count"`
	CheckCount   int `json:"check_count"`
	CreateCount  int `json:"create_count"`
	AdoptCount   int `json:"adopt_count"`
	SkipCount    int `json:"skip_count"`
	ChangedCount int `json:"changed_count"`
	BlockedCount int `json:"blocked_count"`
	FailureCount int `json:"failure_count"`
}

type PlanTarget struct {
	Environment string `json:"environment"`
	Database    string `json:"database"`
}

type PlannedSchema struct {
	SchemaName string `json:"schema_name"`
	Action     string `json:"action"`
	Exists     bool   `json:"exists_in_database"`
}

type PlannedObject struct {
	ObjectPath      string   `json:"object_path"`
	SchemaName      string   `json:"schema_name"`
	Kind            string   `json:"kind"`
	ObjectName      string   `json:"object_name"`
	ParentName      string   `json:"parent_name,omitempty"`
	NormalizedKey   string   `json:"normalized_key"`
	Checksum        string   `json:"checksum"`
	Exists          bool     `json:"exists_in_database"`
	MetadataMatch   *bool    `json:"metadata_match,omitempty"`
	PlannedAction   string   `json:"planned_action"`
	TransactionMode string   `json:"transaction_mode,omitempty"`
	RollbackScope   string   `json:"rollback_scope,omitempty"`
	NoTransaction   bool     `json:"no_transaction,omitempty"`
	SourceFile      string   `json:"source_file,omitempty"`
	TransitionPaths []string `json:"transition_paths,omitempty"`
}

type MigrationPlan struct {
	SchemaVersion     string          `json:"schema_version"`
	Command           string          `json:"command"`
	Tool              string          `json:"tool"`
	ToolVersion       string          `json:"tool_version"`
	ToolCommit        string          `json:"tool_commit"`
	GitCommit         string          `json:"git_commit"`
	GitBranch         string          `json:"git_branch,omitempty"`
	SQLRoot           string          `json:"sql_root"`
	Base              string          `json:"base"`
	EffectiveBasePath string          `json:"effective_base_path"`
	LayoutHash        string          `json:"layout_hash"`
	Target            PlanTarget      `json:"target"`
	ComparisonMode    string          `json:"comparison_mode"`
	UpdatePolicy      string          `json:"update_policy"`
	TransactionMode   string          `json:"transaction_mode"`
	Rollback          string          `json:"rollback"`
	PlannedAt         time.Time       `json:"planned_at"`
	Summary           PlanSummary     `json:"summary"`
	Schemas           []PlannedSchema `json:"schemas"`
	Objects           []PlannedObject `json:"objects"`
	Failures          []string        `json:"failures"`
	Blockers          []string        `json:"blockers"`
	Blocked           bool            `json:"blocked"`
	BlockReasons      []string        `json:"block_reasons,omitempty"`
}

type MigrationReport struct {
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	ToolCommit  string `json:"tool_commit"`
	Environment string `json:"environment"`
	Database    string `json:"database"`

	GitCommit         string `json:"git_commit"`
	GitBranch         string `json:"git_branch"`
	LayoutHash        string `json:"layout_hash,omitempty"`
	SQLRoot           string `json:"sql_root,omitempty"`
	Base              string `json:"base,omitempty"`
	EffectiveBasePath string `json:"effective_base_path,omitempty"`
	PipelineRunID     string `json:"pipeline_run_id"`
	PipelineURL       string `json:"pipeline_url"`
	Actor             string `json:"actor"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`

	Result            string            `json:"result"`
	Applied           []ScriptResult    `json:"applied"`
	Skipped           []ScriptResult    `json:"skipped"`
	Failed            *Failure          `json:"failed,omitempty"`
	ValidationScope   string            `json:"validation_scope,omitempty"`
	ValidationSkipped bool              `json:"validation_skipped,omitempty"`
	Validation        ValidationSummary `json:"validation"`
}

type ValidationReport struct {
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	ToolCommit  string `json:"tool_commit"`
	Environment string `json:"environment"`
	Database    string `json:"database"`

	GitCommit      string `json:"git_commit"`
	GitBranch      string `json:"git_branch"`
	Command        string `json:"command,omitempty"`
	LayoutHash     string `json:"layout_hash,omitempty"`
	SQLRoot        string `json:"sql_root,omitempty"`
	Base           string `json:"base,omitempty"`
	Scope          string `json:"scope,omitempty"`
	IncludesChecks bool   `json:"includes_checks,omitempty"`
	PipelineRunID  string `json:"pipeline_run_id"`
	PipelineURL    string `json:"pipeline_url"`
	Actor          string `json:"actor"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`

	Result     string            `json:"result"`
	Validation ValidationSummary `json:"validation"`
	Failed     *Failure          `json:"failed,omitempty"`
}
