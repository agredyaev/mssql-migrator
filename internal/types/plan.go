package types

import "time"

type PlanTarget struct {
	Environment string
	Database    string
}

type PlanSummary struct {
	SchemaCount  int
	ObjectCount  int
	CheckCount   int
	CreateCount  int
	AdoptCount   int
	SkipCount    int
	ChangedCount int
	BlockedCount int
	FailureCount int
}

type PlannedSchema struct {
	SchemaName string
	Action     string
	Exists     bool
}

type PlannedObject struct {
	Git           *GitInfo `json:"-"`
	MetadataMatch *bool
	ObjectRef
	DatabaseName    string
	ParentName      string
	PlannedAction   string
	TransactionMode string
	RollbackScope   string
	TransitionPaths []string
	Checksum        [32]byte
	Exists          bool
	NoTransaction   bool
}

type MigrationPlan struct {
	PlannedAt         time.Time
	Target            PlanTarget
	ComparisonMode    string
	UpdatePolicy      string
	ToolVersion       string
	ToolCommit        string
	GitCommit         string
	GitBranch         string
	SQLRoot           string
	Base              string
	EffectiveBasePath string
	TransactionMode   string
	Tool              string
	Command           string
	LayoutHash        string
	Rollback          string
	SchemaVersion     string
	Schemas           []PlannedSchema
	Objects           []PlannedObject
	Failures          []string
	Blockers          []string
	BlockReasons      []string
	Summary           PlanSummary
	Blocked           bool
}
