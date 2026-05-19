package db

import "reporting-db-migrations/internal/types"

// InspectScope controls scoped catalog inspection (phase 1: git-delta hot path).
type InspectScope struct {
	// FullInspect runs the legacy full-layout catalog path.
	FullInspect bool
	// HotRefs are layout objects that need a SQL catalog round-trip.
	HotRefs []types.ObjectScopeRef
	// StableObjects are merged into state.Objects without DB lookup (file digest matched history).
	StableObjects map[string]Object
}
