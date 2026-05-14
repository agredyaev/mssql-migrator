package audit

import (
	"context"
	_ "embed"
	"time"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/bootstrap.sql
var bootstrapSQL string

//go:embed sql/insert_attempt.sql
var insertAttemptSQL string

//go:embed sql/update_run.sql
var updateRunSQL string

type Subscriber struct {
	conn  driver.Conn
	runID int64
}

func NewSubscriber(b bus.EventBus, conn driver.Conn) *Subscriber {
	s := &Subscriber{conn: conn}
	b.Subscribe(types.EventRunStarted, s.onRunStarted)
	b.Subscribe(types.EventDiffComputed, s.onDiffComputed)
	b.Subscribe(types.EventObjectApplied, s.onObjectApplied)
	b.Subscribe(types.EventObjectSkipped, s.onObjectSkipped)
	b.Subscribe(types.EventObjectFailed, s.onObjectFailed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) onRunStarted(payload any) {
	ctx := context.Background()
	_, _ = s.conn.ExecContext(ctx, bootstrapSQL)
	now := time.Now().UTC()
	ev := payload.(*types.RunStarted)
	if rows, err := s.conn.QueryContext(ctx, "INSERT INTO __migrator.runs (command, started_at) VALUES ('"+ev.Command+"', @p1); SELECT SCOPE_IDENTITY();", now); err == nil {
		defer rows.Close()
		if rows.Next() {
			rows.Scan(&s.runID)
		}
	}
}

func (s *Subscriber) onDiffComputed(payload any) {
	ctx := context.Background()
	_, _ = s.conn.ExecContext(ctx, "INSERT INTO __migrator.items DEFAULT VALUES")
}

func (s *Subscriber) onObjectApplied(payload any) {
	ctx := context.Background()
	_, _ = s.conn.ExecContext(ctx, insertAttemptSQL,
		s.runID, 0, "k", "cs", "applied", "", time.Now().UTC())
}

func (s *Subscriber) onObjectSkipped(payload any) {
	ctx := context.Background()
	_, _ = s.conn.ExecContext(ctx, insertAttemptSQL,
		s.runID, 0, "k", "cs", "skipped", "", time.Now().UTC())
}

func (s *Subscriber) onObjectFailed(payload any) {
	ctx := context.Background()
	ev := payload.(*types.FailureEvent)
	_, _ = s.conn.ExecContext(ctx, insertAttemptSQL,
		s.runID, 0, ev.NormalizedKey, ev.Checksum, "failed", ev.Error, time.Now().UTC())
}

func (s *Subscriber) onRunFinished(payload any) {
	ctx := context.Background()
	ev := payload.(*types.RunFinished)
	_, _ = s.conn.ExecContext(ctx, updateRunSQL,
		time.Now().UTC(), ev.Result, ev.ExitCode, s.runID)
}
