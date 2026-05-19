//go:build integration

package app

import (
	"context"
	"sync"
	"time"

	"reporting-db-migrations/internal/audit"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/engine"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/prodgate"
	"reporting-db-migrations/internal/types"
)

// PlanPipelineOptions configures RunPlanPipeline (integration / prod gate).
type PlanPipelineOptions struct {
	// EnsureAudit overlaps audit.EnsureTables with inspect (same as engine.runPlan).
	EnsureAudit bool
}

// RunPlanPipeline runs scan → parallel (inspect ‖ ensure+checksums) → diff.
// This matches engine.runPlan.
func RunPlanPipeline(
	ctx context.Context,
	cfg types.Config,
	conn driver.Conn,
	sqlRoot string,
	opts PlanPipelineOptions,
) (*types.MigrationPlan, fs.Layout, prodgate.PhaseTimings, error) {
	var timings prodgate.PhaseTimings
	startAll := time.Now()

	scanner := fs.NewScanner()
	scanner.SkipGit = cfg.SkipGit
	start := time.Now()
	layout, err := scanner.Scan(ctx, sqlRoot)
	if err != nil {
		return nil, fs.Layout{}, timings, err
	}
	timings.ScanMS = prodgate.DurMS(time.Since(start))

	keys := layout.NormalizedKeys()
	var (
		state      *db.State
		checksums  map[string][32]byte
		inspErr    error
		loadErr    error
		ensureErr  error
		inspectDur time.Duration
		loadDur    time.Duration
		ensureDur  time.Duration
	)
	parallelStart := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if opts.EnsureAudit {
			t0 := time.Now()
			ensureErr = audit.EnsureTables(ctx, conn)
			ensureDur = time.Since(t0)
		}
	}()
	go func() {
		defer wg.Done()
		t1 := time.Now()
		checksums, loadErr = audit.LoadChecksums(ctx, conn, keys)
		loadDur = time.Since(t1)
		if loadErr != nil {
			return
		}
		iscope := engine.BuildInspectScope(cfg, layout, checksums)
		t2 := time.Now()
		state, inspErr = db.NewInspector().InspectWithScope(ctx, conn, layout, iscope)
		inspectDur = time.Since(t2)
	}()
	wg.Wait()
	timings.EnsureMS = prodgate.DurMS(ensureDur)
	timings.InspectMS = prodgate.DurMS(inspectDur)
	timings.ChecksumsMS = prodgate.DurMS(loadDur)
	timings.ParallelWallMS = prodgate.DurMS(time.Since(parallelStart))
	timings.AuditMS = timings.EnsureMS + timings.ChecksumsMS
	if ensureErr != nil {
		return nil, layout, timings, ensureErr
	}
	if inspErr != nil {
		return nil, layout, timings, inspErr
	}
	if loadErr != nil {
		return nil, layout, timings, loadErr
	}

	start = time.Now()
	plan, err := diff.NewComputer(cfg).Compute(ctx, layout, state, checksums)
	if err != nil {
		return nil, layout, timings, err
	}
	timings.DiffMS = prodgate.DurMS(time.Since(start))
	timings.PlanWallMS = prodgate.DurMS(time.Since(startAll))

	return plan, layout, timings, nil
}
