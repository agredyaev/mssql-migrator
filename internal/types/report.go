package types

import "time"

type ScriptResult struct {
	Script      string
	Type        string
	Checksum    string
	Reason      string
	ExecutionMS int
}

type Failure struct {
	Script, Batch, Phase, SQLRoot, Base, Class, Reason, SQL, Error string
}

type BaseReport struct {
	StartedAt     time.Time
	FinishedAt    time.Time
	Failed        *Failure
	SQLRoot       string
	PipelineRunID string
	GitCommit     string
	GitBranch     string
	LayoutHash    string
	Tool          string
	Base          string
	Database      string
	PipelineURL   string
	Actor         string
	Environment   string
	ToolCommit    string
	Result        string
	Version       string
	Validation    ValidationResult
}

type MigrationReport struct {
	EffectiveBasePath string
	ValidationScope   string
	Applied           []ScriptResult
	Skipped           []ScriptResult
	BaseReport
	DurationMS        int64
	ValidationSkipped bool
}

type ValidationReport struct {
	Command string
	Scope   string
	BaseReport
	IncludesChecks bool
}
