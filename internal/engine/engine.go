package engine

import (
	"context"
	"time"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/errors"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

type Engine struct {
	cfg      types.Config
	bus      bus.EventBus
	conn     driver.Conn
	fs       Scanner
	db       Inspector
	load     Loader
	diff     Computer
	scaffold Scaffolder
	applier  Applier
	locker   Locker
}

type Scanner interface {
	Scan(ctx context.Context, root string) (fs.Layout, error)
}

type Inspector interface {
	Inspect(ctx context.Context, conn driver.Conn, scope fs.Layout) (*db.State, error)
}

type Loader interface {
	LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string]string, error)
}

type Computer interface {
	Compute(ctx context.Context, layout fs.Layout, state *db.State, checksums map[string]string) (*types.MigrationPlan, error)
}

type Scaffolder interface {
	Ensure(ctx context.Context, cfg types.Config, layout fs.Layout, plan *types.MigrationPlan, columns map[string][]db.TableColumn) (bool, error)
}

type Applier interface {
	Execute(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, layout fs.Layout, eb bus.EventBus) (*ApplyResult, error)
}

type Locker interface {
	Acquire(ctx context.Context, conn driver.Conn, timeout time.Duration) error
	Release(ctx context.Context, conn driver.Conn) error
}

type ApplyResult = struct{ Applied int }

func New(cfg types.Config, bus bus.EventBus, conn driver.Conn, fs Scanner, db Inspector, load Loader, diff Computer, scaffolder Scaffolder, applier Applier, locker Locker) *Engine {
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

func (e *Engine) Plan(ctx context.Context) error {
	e.bus.Publish(types.EventRunStarted, &types.RunStarted{Command: "plan"})

	plan, layout, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed("plan", err)
		return err
	}
	_ = layout

	e.bus.Publish(types.EventDiffComputed, &types.DiffResult{Plan: plan})
	e.bus.Publish(types.EventRunFinished, &types.RunFinished{Command: "plan", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) Migrate(ctx context.Context) error {
	e.bus.Publish(types.EventRunStarted, &types.RunStarted{Command: "migrate"})

	plan, layout, state, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed("migrate", err)
		return err
	}

	e.bus.Publish(types.EventDiffComputed, &types.DiffResult{Plan: plan})

	if err := e.locker.Acquire(ctx, e.conn, e.cfg.LockTimeout); err != nil {
		e.publishRunFailed("migrate", err)
		return err
	}
	defer e.locker.Release(ctx, e.conn)

	if plan.Blocked {
		e.scaffold.Ensure(ctx, e.cfg, layout, plan, state.TableColumns)
		e.publishRunFailed("migrate", errors.ErrPlanBlocked)
		return errors.ErrPlanBlocked
	}

	_, err = e.applier.Execute(ctx, e.conn, *plan, layout, e.bus)
	if err != nil {
		e.publishRunFailed("migrate", err)
		return err
	}

	e.bus.Publish(types.EventRunFinished, &types.RunFinished{Command: "migrate", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) Validate(ctx context.Context) error {
	e.bus.Publish(types.EventRunStarted, &types.RunStarted{Command: "validate"})
	e.bus.Publish(types.EventValidationStart, &types.ValidationEvent{})

	plan, _, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed("validate", err)
		return err
	}

	e.bus.Publish(types.EventValidationDone, &types.ValidationResult{ModulesRefreshed: plan.Summary.ChangedCount})
	e.bus.Publish(types.EventRunFinished, &types.RunFinished{Command: "validate", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) Baseline(ctx context.Context) error {
	e.bus.Publish(types.EventRunStarted, &types.RunStarted{Command: "baseline"})

	plan, layout, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed("baseline", err)
		return err
	}

	if err := e.locker.Acquire(ctx, e.conn, e.cfg.LockTimeout); err != nil {
		e.publishRunFailed("baseline", err)
		return err
	}
	defer e.locker.Release(ctx, e.conn)

	_, err = e.applier.Execute(ctx, e.conn, *plan, layout, e.bus)
	if err != nil {
		e.publishRunFailed("baseline", err)
		return err
	}

	e.bus.Publish(types.EventRunFinished, &types.RunFinished{Command: "baseline", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) RepairChecksum(ctx context.Context) error {
	e.bus.Publish(types.EventRunStarted, &types.RunStarted{Command: "repair"})

	plan, layout, _, err := e.runPlan(ctx)
	if err != nil {
		e.publishRunFailed("repair", err)
		return err
	}

	if err := e.locker.Acquire(ctx, e.conn, e.cfg.LockTimeout); err != nil {
		e.publishRunFailed("repair", err)
		return err
	}
	defer e.locker.Release(ctx, e.conn)

	_, err = e.applier.Execute(ctx, e.conn, *plan, layout, e.bus)
	if err != nil {
		e.publishRunFailed("repair", err)
		return err
	}

	e.bus.Publish(types.EventRunFinished, &types.RunFinished{Command: "repair", Result: "success", ExitCode: 0})
	return nil
}

func (e *Engine) runPlan(ctx context.Context) (*types.MigrationPlan, fs.Layout, *db.State, error) {
	layout, err := e.fs.Scan(ctx, e.cfg.SQLRoot)
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}

	state, err := e.db.Inspect(ctx, e.conn, layout)
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}

	checksums, err := e.load.LoadChecksums(ctx, e.conn, layout.NormalizedKeys())
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}

	plan, err := e.diff.Compute(ctx, layout, state, checksums)
	if err != nil {
		return nil, fs.Layout{}, nil, err
	}

	return plan, layout, state, nil
}

func (e *Engine) publishRunFailed(command string, err error) {
	e.bus.Publish(types.EventRunFinished, &types.RunFinished{
		Command:  command,
		Result:   "failure",
		ExitCode: 1,
	})
}
