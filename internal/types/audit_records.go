package types

import "time"

type RunRecord struct {
	StartedAt time.Time
	Command   string
	Status    string
	ID        int64
	ExitCode  int
}

type ItemRecord struct {
	NormalizedKey string
	PlannedAction string
	Checksum      string
	ID            int64
	RunID         int64
}

type AttemptRecord struct {
	AttemptedAt   time.Time
	NormalizedKey string
	Checksum      string
	Action        string
	Error         string
	ID            int64
	RunID         int64
	ItemID        int64
}

type ItemResult struct {
	NormalizedKey string
	Action        string
	Error         string
}
