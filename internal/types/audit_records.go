package types

import "time"

type RunRecord struct {
	ID        int64
	Command   string
	StartedAt time.Time
	Status    string
	ExitCode  int
}

type ItemRecord struct {
	ID            int64
	RunID         int64
	NormalizedKey string
	PlannedAction string
	Checksum      string
}

type AttemptRecord struct {
	ID            int64
	RunID         int64
	ItemID        int64
	NormalizedKey string
	Checksum      string
	Action        string
	Error         string
	AttemptedAt   time.Time
}

type ItemResult struct {
	NormalizedKey string
	Action        string
	Error         string
}
