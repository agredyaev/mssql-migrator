package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"reporting-db-migrations/internal/apply"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/errors"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/lock"
	"reporting-db-migrations/internal/types"
)

type BootstrapChecker interface {
	BootstrapError() error
}

// PhaseObserver receives wall durations for engine sub-phases (scan, inspect, checksums, diff, …).
// Used by integration profiling; nil is a no-op.
type PhaseObserver func(phase string, d time.Duration)

type Engine struct {
	diff          Computer
	bus           bus.EventBus
	conn          driver.Conn
	fs            Scanner
	db            Inspector
	load          Loader
	scaffold      Scaffolder
	applier       Applier
	locker        lock.Locker
	bc            BootstrapChecker
	cfg           types.Config
	phaseObserver PhaseObserver
}

type Scanner interface {
	Scan(ctx context.Context, root string) (fs.Layout, error)
}

type Inspector interface {
	Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*db.State, error)
	InspectWithScope(ctx context.Context, conn driver.Conn, layout fs.Layout, iscope db.InspectScope) (*db.State, error)
	LoadTableColumns(ctx context.Context, conn driver.Conn, scope fs.Layout) (map[string][]db.TableColumn, error)
}

type Loader interface {
	EnsureTables(ctx context.Context, conn driver.Conn) error
	LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string][32]byte, error)
	LoadAllAppliedMigrations(ctx context.Context, conn driver.Conn) (map[string]bool, error)
}

type Computer interface {
	Compute(ctx context.Context, layout fs.Layout, state *db.State, checksums map[string][32]byte) (*types.MigrationPlan, error)
}

type Scaffolder interface {
	Ensure(ctx context.Context, cfg types.Config, layout fs.Layout, plan *types.MigrationPlan, columns map[string][]db.TableColumn) (bool, error)
}

type Applier interface {
	Execute(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, layout fs.Layout, eb bus.EventBus) (*apply.ApplyResult, error)
}

func New(cfg types.Config, bus bus.EventBus, conn driver.Conn, fs Scanner, db Inspector, load Loader, diff Computer, scaffolder Scaffolder, applier Applier, locker lock.Locker) *Engine {
	return &Engine{
		cfg:      cfg,
		bus:      bus,
		conn:     conn,
		fs:       fs,
		db:       db,
		load:     load,
		diff:     diff,
		scaffold: scaffolder,
		applier:  applier,
		locker:   locker,
	}
}

func (e *Engine) SetBootstrapChecker(bc BootstrapChecker) {
	e.bc = bc
}

func (e *Engine) SetPhaseObserver(o PhaseObserver) {
	e.phaseObserver = o
}

func (e *Engine) observePhase(phase string, d time.Duration) {
	if e.phaseObserver != nil {
		e.phaseObserver(phase, d)
	}
}

// RunPlan executes scan → parallel inspect/checksums → diff without publishing bus events.
func (e *Engine) RunPlan(ctx context.Context) (*types.MigrationPlan, fs.Layout, *db.State, error) {
	return e.runPlan(ctx)
}

func (e *Engine) Plan(ctx context.Context) error {
	start := time.Now()
	defer func() { e.observePhase("engine", time.Since(start)) }()

	e.bus.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "plan"})

	plan, layout, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed(ctx, "plan", err)
		return err
	}
	_ = layout

	e.bus.Publish(ctx, types.EventDiffComputed, &types.DiffResult{Plan: plan})
	e.bus.Publish(ctx, types.EventRunFinished, &types.RunFinished{Command: "plan", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) Migrate(ctx context.Context) error {
	start := time.Now()
	defer func() { e.observePhase("engine", time.Since(start)) }()

	e.bus.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "migrate"})

	plan, layout, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed(ctx, "migrate", err)
		return err
	}

	e.bus.Publish(ctx, types.EventDiffComputed, &types.DiffResult{Plan: plan})

	if plan.Blocked {
		columns, err := e.db.LoadTableColumns(ctx, e.conn, layout)
		if err != nil {
			e.publishRunFailed(ctx, "migrate", err)
			return err
		}
		if _, err := e.scaffold.Ensure(ctx, e.cfg, layout, plan, columns); err != nil {
			e.publishRunFailed(ctx, "migrate", err)
			return err
		}
		e.publishRunFailed(ctx, "migrate", errors.ErrPlanBlocked)
		return errors.ErrPlanBlocked
	}

	if err := e.locker.Acquire(ctx, e.conn, e.cfg.LockTimeout); err != nil {
		e.publishRunFailed(ctx, "migrate", err)
		return err
	}
	defer e.locker.Release(ctx, e.conn)

	if err := e.filterAppliedMigrations(ctx, plan); err != nil {
		e.publishRunFailed(ctx, "migrate", err)
		return err
	}

	applyStart := time.Now()
	res, err := e.applier.Execute(ctx, e.conn, *plan, layout, e.bus)
	e.observePhase("apply", time.Since(applyStart))
	if err != nil {
		e.publishRunFailed(ctx, "migrate", err)
		return err
	}
	if res.Failed > 0 {
		msg := fmt.Sprintf("%d object(s) failed to apply: %s", res.Failed, strings.Join(res.Errors, "; "))
		e.publishRunFailed(ctx, "migrate", errors.ErrSQLExecution)
		return fmt.Errorf("%s", msg)
	}
	if e.bc != nil {
		if berr := e.bc.BootstrapError(); berr != nil {
			e.publishRunFailed(ctx, "migrate", berr)
			return berr
		}
	}

	e.bus.Publish(ctx, types.EventRunFinished, &types.RunFinished{Command: "migrate", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) filterAppliedMigrations(ctx context.Context, plan *types.MigrationPlan) error {
	needLookup := false
	for i := range plan.Objects {
		obj := &plan.Objects[i]
		if obj.PlannedAction == types.ActionReprocessChanged && len(obj.TransitionPaths) > 0 {
			needLookup = true
			break
		}
	}
	if !needLookup {
		return nil
	}

	applied, err := e.load.LoadAllAppliedMigrations(ctx, e.conn)
	if err != nil {
		return err
	}

	for i := range plan.Objects {
		obj := &plan.Objects[i]
		if obj.PlannedAction != types.ActionReprocessChanged || len(obj.TransitionPaths) == 0 {
			continue
		}
		filtered := make([]string, 0, len(obj.TransitionPaths))
		for _, tp := range obj.TransitionPaths {
			if !applied[tp] {
				filtered = append(filtered, tp)
			}
		}
		obj.TransitionPaths = filtered
	}
	return nil
}

// Validate runs the same planning pipeline as plan (scan, inspect without table
// columns, checksum load, diff) and reports changed module count. It does not
// execute layout.Checks SQL scripts; use a dedicated checks runner if needed.
func (e *Engine) Validate(ctx context.Context) error {
	e.bus.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "validate"})
	e.bus.Publish(ctx, types.EventValidationStart, &types.ValidationEvent{})

	plan, _, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed(ctx, "validate", err)
		return err
	}

	e.bus.Publish(ctx, types.EventValidationDone, &types.ValidationResult{ModulesRefreshed: plan.Summary.ChangedCount})
	e.bus.Publish(ctx, types.EventRunFinished, &types.RunFinished{Command: "validate", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) Baseline(ctx context.Context) error {
	e.bus.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "baseline"})
	return e.executeLocked(ctx, "baseline")
}

func (e *Engine) RepairChecksum(ctx context.Context) error {
	e.bus.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "repair"})
	return e.executeLocked(ctx, "repair")
}

func (e *Engine) executeLocked(ctx context.Context, command string) error {
	plan, layout, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed(ctx, command, err)
		return err
	}

	if err := e.locker.Acquire(ctx, e.conn, e.cfg.LockTimeout); err != nil {
		e.publishRunFailed(ctx, command, err)
		return err
	}
	defer e.locker.Release(ctx, e.conn)

	res, err := e.applier.Execute(ctx, e.conn, *plan, layout, e.bus)
	if err != nil {
		e.publishRunFailed(ctx, command, err)
		return err
	}
	if res.Failed > 0 {
		msg := fmt.Sprintf("%d object(s) failed to apply: %s", res.Failed, strings.Join(res.Errors, "; "))
		e.publishRunFailed(ctx, command, errors.ErrSQLExecution)
		return fmt.Errorf("%s", msg)
	}
	if e.bc != nil {
		if berr := e.bc.BootstrapError(); berr != nil {
			e.publishRunFailed(ctx, command, berr)
			return berr
		}
	}

	e.bus.Publish(ctx, types.EventRunFinished, &types.RunFinished{Command: command, Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) runPlan(ctx context.Context) (*types.MigrationPlan, fs.Layout, *db.State, error) {
	start := time.Now()
	layout, err := e.fs.Scan(ctx, e.cfg.SQLRoot)
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}
	e.observePhase("scan", time.Since(start))

	keys := layout.NormalizedKeys()
	var (
		state      *db.State
		checksums  map[string][32]byte
		inspErr    error
		loadErr    error
		ensureErr  error
		inspectDur time.Duration
		loadDur    time.Duration
	)
	parallelStart := time.Now()
	var ensureDur time.Duration
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		if ensureErr = e.load.EnsureTables(ctx, e.conn); ensureErr != nil {
			return
		}
		ensureDur = time.Since(t0)
	}()
	go func() {
		defer wg.Done()
		t1 := time.Now()
		checksums, loadErr = e.load.LoadChecksums(ctx, e.conn, keys)
		loadDur = time.Since(t1)
		if loadErr != nil {
			return
		}
		iscope := e.buildInspectScope(layout, checksums)
		t2 := time.Now()
		state, inspErr = e.db.InspectWithScope(ctx, e.conn, layout, iscope)
		inspectDur = time.Since(t2)
	}()
	wg.Wait()
	e.observePhase("ensure", ensureDur)
	e.observePhase("inspect", inspectDur)
	e.observePhase("checksums", loadDur)
	e.observePhase("parallel_wall", time.Since(parallelStart))
	if inspErr != nil {
		return nil, fs.Layout{}, nil, inspErr
	}
	if ensureErr != nil {
		return nil, fs.Layout{}, nil, ensureErr
	}
	if loadErr != nil {
		return nil, fs.Layout{}, nil, loadErr
	}

	start = time.Now()
	plan, err := e.diff.Compute(ctx, layout, state, checksums)
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}
	e.observePhase("diff", time.Since(start))

	return plan, layout, state, nil
}

func (e *Engine) publishRunFailed(ctx context.Context, command string, err error) {
	e.bus.Publish(ctx, types.EventRunFinished, &types.RunFinished{
		Command:  command,
		Result:   "failure",
		ExitCode: 1,
	})
}
