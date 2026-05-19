package engine

import (
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/plan"
	"reporting-db-migrations/internal/prodgate"
	"reporting-db-migrations/internal/types"
)

// BuildInspectScope resolves git delta and classifies layout objects for scoped catalog SQL.
func BuildInspectScope(cfg types.Config, layout fs.Layout, checksums map[string][32]byte) db.InspectScope {
	if cfg.SkipGit {
		return db.InspectScope{FullInspect: true}
	}
	pathsResult, err := prodgate.ResolveChangedPaths(cfg.SQLRoot)
	full := pathsResult.FullInspect
	if err != nil {
		full = true
	}
	return plan.BuildInspectScope(layout, pathsResult.Paths, full, checksums)
}

func (e *Engine) buildInspectScope(layout fs.Layout, checksums map[string][32]byte) db.InspectScope {
	return BuildInspectScope(e.cfg, layout, checksums)
}
