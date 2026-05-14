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

type ValidationSummary struct {
	ModulesRefreshed int
	ChecksPassed     int
	ChecksFailed     int
}

type MigrationReport struct {
	Tool, Version, ToolCommit, Environment, Database string
	GitCommit, GitBranch, LayoutHash                 string
	SQLRoot, Base, EffectiveBasePath                 string
	PipelineRunID, PipelineURL, Actor                string
	StartedAt, FinishedAt                            time.Time
	DurationMS                                       int64
	Result                                           string
	Applied                                          []ScriptResult
	Skipped                                          []ScriptResult
	Failed                                           *Failure
	ValidationScope                                  string
	ValidationSkipped                                bool
	Validation                                       ValidationSummary
}

type ValidationReport struct {
	Tool, Version, ToolCommit, Environment, Database string
	GitCommit, GitBranch, Command, LayoutHash        string
	SQLRoot, Base, Scope                             string
	IncludesChecks                                   bool
	PipelineRunID, PipelineURL, Actor                string
	StartedAt, FinishedAt                            time.Time
	Result                                           string
	Validation                                       ValidationSummary
	Failed                                           *Failure
}
