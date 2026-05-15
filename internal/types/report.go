package types

import "time"

type ScriptResult struct {
	Script, Type, Checksum string
	ExecutionMS            int
	Reason                 string
}

type Failure struct {
	Script, Batch, Phase, SQLRoot, Base, Class, Reason, SQL, Error string
}

type BaseReport struct {
	Tool, Version, ToolCommit, Environment, Database string
	GitCommit, GitBranch, LayoutHash                 string
	SQLRoot, Base                                    string
	PipelineRunID, PipelineURL, Actor                string
	StartedAt, FinishedAt                            time.Time
	Result                                           string
	Validation                                       ValidationResult
	Failed                                           *Failure
}

type MigrationReport struct {
	BaseReport
	EffectiveBasePath string
	DurationMS        int64
	Applied           []ScriptResult
	Skipped           []ScriptResult
	ValidationScope   string
	ValidationSkipped bool
}

type ValidationReport struct {
	BaseReport
	Command        string
	Scope          string
	IncludesChecks bool
}
