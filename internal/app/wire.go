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

func (loaderAdapter) LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string][32]byte, error) {
	return audit.LoadChecksums(ctx, conn, keys)
}

func (loaderAdapter) LoadAllAppliedMigrations(ctx context.Context, conn driver.Conn) (map[string]bool, error) {
	return audit.LoadAllAppliedMigrations(ctx, conn)
}

func attachSubscribers(b bus.EventBus, conn driver.Conn, cfg types.Config, logger *log.Logger) *audit.Subscriber {
	auditSub := audit.NewSubscriber(b, conn)
	auditSub.SetErrorHandler(func(msg string) { logger.Warn("audit", msg) })

	reportSub := report.NewSubscriber(b, cfg)
	reportSub.SetErrorHandler(func(msg string) { logger.Warn("report", msg) })

	return auditSub
}

func wireEngine(b bus.EventBus, conn driver.Conn, cfg types.Config, logger *log.Logger) *engine.Engine {
	scanner := fs.NewScanner()
	scanner.SkipGit = cfg.SkipGit
	return engine.New(
		cfg,
		b,
		conn,
		scanner,
		db.NewInspector(),
		loaderAdapter{},
		diff.NewComputer(cfg),
		scaffold.New(),
		apply.New(),
		lock.New(),
	)
}
