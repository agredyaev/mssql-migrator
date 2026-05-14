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
	NormalizedKey   string
	ObjectPath      string
	SchemaName      string
	Kind            string
	ObjectName      string
	ParentName      string
	Checksum        string
	PlannedAction   string
	TransactionMode string
	RollbackScope   string
	SourceFile      string
	TransitionPaths []string
	MetadataMatch   *bool
	Exists          bool
	NoTransaction   bool
}

type MigrationPlan struct {
	PlannedAt         time.Time
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
	ComparisonMode    string
	UpdatePolicy      string
	TransactionMode   string
	Rollback          string
	Target            PlanTarget
	Summary           PlanSummary
	Schemas           []PlannedSchema
	Objects           []PlannedObject
	Failures          []string
	Blockers          []string
	BlockReasons      []string
	Blocked           bool
}
