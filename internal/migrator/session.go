package migrator

import (
	"context"
	"database/sql"
	"fmt"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
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
	if err := s.requireConnection(); err != nil {
		return err
	}
	return bootstrapMetadata(ctx, s.conn)
}

func (s *runSession) LoadSuccessfulChecksums(ctx context.Context) (map[string]string, error) {
	if err := s.requireConnection(); err != nil {
		return nil, err
	}
	return metadata.LoadSuccessfulChecksumsIfPresent(ctx, s.conn)
}

func (s *runSession) StartRun(ctx context.Context, command string, planFile string, planHash string, rollbackScope string) (string, metadataRecorder, error) {
	if err := s.requireConnection(); err != nil {
		return "", metadataRecorder{}, err
	}
	runID, err := s.runner.startRun(ctx, s.conn, command, planFile, planHash, rollbackScope)
	if err != nil {
		return "", metadataRecorder{}, err
	}
	return runID, newMetadataRecorder(s.runner.cfg, s.conn, s.conn, runID), nil
}

func (s *runSession) Recorder(runID string) metadataRecorder {
	if s == nil || s.conn == nil {
		return metadataRecorder{}
	}
	return newMetadataRecorder(s.runner.cfg, s.conn, s.conn, runID)
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
	return FailureReporter{cfg: s.runner.cfg}.Migration(s.report, base, cause)
}

func (s *runSession) ResolvePlanningLayout() (parser.Layout, string, error) {
	return planner.ResolvePlanningLayoutForRunner(s.runner.cfg)
}

func (s *runSession) BuildPlan(ctx context.Context, successfulByKey map[string]string, layout parser.Layout, hash string) (contracts.MigrationPlan, error) {
	if err := s.requireConnection(); err != nil {
		return contracts.MigrationPlan{}, err
	}
	return planner.BuildResolved(ctx, s.runner.cfg, successfulByKey, layout, hash, planner.SQLCatalogReader(s.conn))
}

func (s *runSession) ReadPlanningCatalog(ctx context.Context) (planner.CatalogState, error) {
	if err := s.requireConnection(); err != nil {
		return planner.CatalogState{}, err
	}
	return planner.SQLCatalogReader(s.conn).ReadCatalogState(ctx)
}

func (s *runSession) RecordRunFailure(ctx context.Context, recorder metadataRecorder, base error, cause error) {
	s.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, base, cause))
}

func (s *runSession) FinishRun(ctx context.Context, recorder metadataRecorder) error {
	return recorder.recordRunResult(ctx, true, nil, nil)
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

func (s *runSession) requireConnection() error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("protected session connection is unavailable")
	}
	return nil
}
