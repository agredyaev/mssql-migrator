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
	ObjectPath      string
	SchemaName      string
	Kind            string
	ObjectName      string
	ParentName      string
	NormalizedKey   string
	Checksum        string
	Exists          bool
	MetadataMatch   *bool
	PlannedAction   string
	TransactionMode string
	RollbackScope   string
	NoTransaction   bool
	SourceFile      string
	TransitionPaths []string
}

type MigrationPlan struct {
	SchemaVersion     string
	Command           string
	Tool              string
	ToolVersion       string
	ToolCommit        string
	GitCommit         string
	GitBranch         string
	SQLRoot           string
	Base              string
	EffectiveBasePath string
	LayoutHash        string
	Target            PlanTarget
	ComparisonMode    string
	UpdatePolicy      string
	TransactionMode   string
	Rollback          string
	PlannedAt         time.Time
	Summary           PlanSummary
	Schemas           []PlannedSchema
	Objects           []PlannedObject
	Failures          []string
	Blockers          []string
	Blocked           bool
	BlockReasons      []string
}
