package migrator

import (
	"context"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/runreport"
)

type protectedRunState struct {
	phase    string
	session  *runSession
	runID    string
	recorder metadataRecorder
}

func (r Runner) startProtectedRunState(ctx context.Context, phase string) (*protectedRunState, error) {
	session, err := r.startProtectedSession(ctx)
	if err != nil {
		return nil, session.Fail(phase, err, nil)
	}
	return &protectedRunState{phase: phase, session: session, recorder: session.Recorder("")}, nil
}

func (s *protectedRunState) Close() {
	if s == nil {
		return
	}
	s.session.Close()
}

func (s *protectedRunState) setLayoutHash(hash string) {
	if s == nil {
		return
	}
	s.session.SetLayoutHash(hash)
}

func (s *protectedRunState) startRun(ctx context.Context, command string, planFile string, planHash string, rollback string) error {
	runID, recorder, err := s.session.StartRun(ctx, command, planFile, planHash, rollback)
	if err != nil {
		return err
	}
	s.runID = runID
	s.recorder = recorder
	return nil
}

func (s *protectedRunState) recordRunFailure(ctx context.Context, base error, cause error) {
	if s == nil || s.runID == "" {
		return
	}
	s.session.RecordRunFailure(ctx, s.recorder, base, cause)
}

func (s *protectedRunState) fail(ctx context.Context, base error, cause error) error {
	s.recordRunFailure(ctx, base, cause)
	return s.session.Fail(s.phase, base, cause)
}

func (s *protectedRunState) finishSuccess(ctx context.Context) error {
	runreport.FinalizeMigrationSuccess(s.session.MigrationReport())
	if err := s.session.FinishRun(ctx, s.recorder); err != nil {
		return s.session.Fail(s.phase, contracts.ErrCriticalState, err)
	}
	return s.session.WriteMigrationReport()
}

func (s *protectedRunState) failAfterReportWrite(ctx context.Context, err error) error {
	if writeErr := s.session.WriteMigrationReport(); writeErr != nil {
		return contracts.Wrap(contracts.ErrCriticalState, writeErr)
	}
	s.recordRunFailure(ctx, err, nil)
	return err
}

func (s *protectedRunState) executeTrackedPlan(ctx context.Context, runner Runner, layout parser.Layout, plan contracts.MigrationPlan, itemIDs map[string]int64) error {
	if err := runner.executePlanTracked(ctx, s.session.conn, layout, plan, s.session.MigrationReport(), s.runID, itemIDs); err != nil {
		return s.failAfterReportWrite(ctx, err)
	}
	return nil
}
