package types

// Event is a bus event constant.
type Event string

const (
	EventRunStarted        Event = "run.started"
	EventRunFinished       Event = "run.finished"
	EventDiffComputed      Event = "diff.computed"
	EventScaffoldGenerated Event = "scaffold.generated"
	EventSchemaCreated     Event = "schema.created"
	EventObjectSkipped     Event = "object.skipped"
	EventObjectApplied     Event = "object.applied"
	EventObjectFailed      Event = "object.failed"
	EventValidationStart   Event = "validation.start"
	EventValidationDone    Event = "validation.done"
)

type RunStarted struct {
	Command string
}

type RunFinished struct {
	Command  string
	Result   string
	ExitCode int
}

type DiffResult struct {
	Plan *MigrationPlan
}

type ScaffoldResult struct {
	Paths []string
}

type SchemaEvent struct {
	SchemaName string
	Action     string
}

type ObjectEvent struct {
	ObjectRef

	Checksum   string
	Action     string
	Path       string
	RecordKind string

	GitInfo
}

type FailureEvent struct {
	ObjectEvent
	Error string
	SQL   string
}

type ValidationEvent struct {
	Scope string
}

type ValidationResult struct {
	ModulesRefreshed int
	ChecksPassed     int
	ChecksFailed     int
}
