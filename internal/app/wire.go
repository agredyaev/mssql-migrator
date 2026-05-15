package app

import (
	"context"

	"reporting-db-migrations/internal/apply"
	"reporting-db-migrations/internal/audit"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/engine"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/lock"
	"reporting-db-migrations/internal/log"
	"reporting-db-migrations/internal/report"
	"reporting-db-migrations/internal/scaffold"
	"reporting-db-migrations/internal/types"
)

type loaderAdapter struct{}

func (loaderAdapter) EnsureTables(ctx context.Context, conn driver.Conn) error {
	return audit.EnsureTables(ctx, conn)
}

func (loaderAdapter) LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string]string, error) {
	return audit.LoadChecksums(ctx, conn, keys)
}

func (loaderAdapter) LoadAppliedMigrations(ctx context.Context, conn driver.Conn, tableKey string) (map[string]bool, error) {
	return audit.LoadAppliedMigrations(ctx, conn, tableKey)
}

type applierAdapter struct {
	exec *apply.Executor
}

func (a *applierAdapter) Execute(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, layout fs.Layout, eb bus.EventBus) (*engine.ApplyResult, error) {
	result, err := a.exec.Execute(ctx, conn, plan, layout, eb)
	if err != nil {
		return nil, err
	}
	return &engine.ApplyResult{Applied: result.Applied}, nil
}

func attachSubscribers(b bus.EventBus, conn driver.Conn, cfg types.Config, logger *log.Logger) {
	auditSub := audit.NewSubscriber(b, conn)
	auditSub.SetErrorHandler(func(msg string) { logger.Warn("audit", msg) })

	reportSub := report.NewSubscriber(b, cfg)
	reportSub.SetErrorHandler(func(msg string) { logger.Warn("report", msg) })
}

func wireEngine(b bus.EventBus, conn driver.Conn, cfg types.Config, logger *log.Logger) *engine.Engine {
	return engine.New(
		cfg,
		b,
		conn,
		fs.NewScanner(),
		db.NewInspector(),
		loaderAdapter{},
		diff.NewComputer(),
		scaffold.New(),
		&applierAdapter{exec: apply.New()},
		lock.New(),
	)
}
