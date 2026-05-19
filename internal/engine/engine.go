package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

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

type Engine struct {
	diff     Computer
	bus      bus.EventBus
	conn     driver.Conn
	fs       Scanner
	db       Inspector
	load     Loader
	scaffold Scaffolder
	applier  Applier
	locker   lock.Locker
	bc       BootstrapChecker
	cfg      types.Config
}

type Scanner interface {
	Scan(ctx context.Context, root string) (fs.Layout, error)
}

type Inspector interface {
	Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*db.State, error)
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

func (e *Engine) Plan(ctx context.Context) error {
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

	res, err := e.applier.Execute(ctx, e.conn, *plan, layout, e.bus)
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
	layout, err := e.fs.Scan(ctx, e.cfg.SQLRoot)
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}

	keys := layout.NormalizedKeys()
	var (
		state     *db.State
		checksums map[string][32]byte
		inspErr   error
		loadErr   error
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		state, inspErr = e.db.Inspect(ctx, e.conn, layout)
	}()
	go func() {
		defer wg.Done()
		checksums, loadErr = e.load.LoadChecksums(ctx, e.conn, keys)
	}()
	wg.Wait()
	if inspErr != nil {
		return nil, fs.Layout{}, nil, inspErr
	}
	if loadErr != nil {
		return nil, fs.Layout{}, nil, loadErr
	}

	plan, err := e.diff.Compute(ctx, layout, state, checksums)
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}

	return plan, layout, state, nil
}

func (e *Engine) publishRunFailed(ctx context.Context, command string, err error) {
	e.bus.Publish(ctx, types.EventRunFinished, &types.RunFinished{
		Command:  command,
		Result:   "failure",
		ExitCode: 1,
	})
}
