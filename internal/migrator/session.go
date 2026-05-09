package migrator

import (
	"context"
	"database/sql"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
)

type runSession struct {
	runner  Runner
	report  contracts.MigrationReport
	conn    *sql.Conn
	closeFn func() error
}

func (r Runner) startProtectedSession(ctx context.Context) (*runSession, error) {
	report, conn, closeFn, err := r.prepareProtectedRun(ctx)
	if err != nil {
		return &runSession{runner: r, report: report}, err
	}
	session := &runSession{runner: r, report: report, conn: conn, closeFn: closeFn}
	if err := r.acquireLock(ctx, conn); err != nil {
		return session, err
	}
	return session, nil
}

func (s *runSession) BootstrapMetadata(ctx context.Context) error {
	if s == nil || s.conn == nil {
		return nil
	}
	return bootstrapMetadata(ctx, s.conn)
}

func (s *runSession) Close() {
	if s == nil || s.closeFn == nil {
		return
	}
	if err := s.closeFn(); err != nil {
		s.runner.log.Warn("db_close_failed", err.Error())
	}
}

func (s *runSession) Fail(base error, cause error) error {
	return s.runner.writeFailedMigration(s.report, base, cause)
}

func (s *runSession) ResolvePlanningLayout() (parser.Layout, string, error) {
	return planner.ResolvePlanningLayoutForRunner(s.runner.cfg)
}

func (s *runSession) SetLayoutHash(hash string) {
	s.report.LayoutHash = hash
}

func (s *runSession) MigrationReport() *contracts.MigrationReport {
	return &s.report
}

func (s *runSession) WriteMigrationReport() error {
	return writeMigrationReport(s.runner.cfg.ReportDir, s.report)
}

func (s *runSession) WarnMetadataFinishFailure(err error) {
	if err != nil {
		s.runner.log.Warn("metadata_finish_run_failed", logger.Redact(err.Error()))
	}
}
