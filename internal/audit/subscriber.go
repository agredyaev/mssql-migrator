package audit

import (
	"context"
	_ "embed"
	"strings"
	"time"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/bootstrap.sql
var bootstrapSQL string

//go:embed sql/insert_run.sql
var insertRunSQL string

//go:embed sql/insert_attempt.sql
var insertAttemptSQL string

//go:embed sql/update_run.sql
var updateRunSQL string

//go:embed sql/insert_items.sql
var insertItemsSQL string

type Subscriber struct {
	conn  driver.Conn
	runID int64
	errf  func(msg string)
}

func NewSubscriber(b bus.EventBus, conn driver.Conn) *Subscriber {
	s := &Subscriber{conn: conn}
	s.errf = func(msg string) {} // no-op by default; engine can override
	b.Subscribe(types.EventRunStarted, s.onRunStarted)
	b.Subscribe(types.EventDiffComputed, s.onDiffComputed)
	b.Subscribe(types.EventObjectApplied, s.onObjectApplied)
	b.Subscribe(types.EventObjectSkipped, s.onObjectSkipped)
	b.Subscribe(types.EventObjectFailed, s.onObjectFailed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.errf = fn
}

func (s *Subscriber) onRunStarted(payload any) {
	ctx := context.Background()
	if _, err := s.conn.ExecContext(ctx, bootstrapSQL); err != nil {
		s.errf("audit bootstrap: " + err.Error())
		return
	}

	ev := payload.(*types.RunStarted)
	now := time.Now().UTC()
	rows, err := s.conn.QueryContext(ctx, insertRunSQL, ev.Command, now)
	if err != nil {
		s.errf("audit insert_run: " + err.Error())
		return
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&s.runID); err != nil {
			s.errf("audit scan run_id: " + err.Error())
		}
	}
}

func (s *Subscriber) onDiffComputed(payload any) {
	ctx := context.Background()
	result := payload.(*types.DiffResult)
	if result.Plan == nil {
		return
	}

	q := strings.Replace(insertItemsSQL, "{{values}}", "(@p1, @p2, @p3, @p4)", 1)
	for _, obj := range result.Plan.Objects {
		if _, err := s.conn.ExecContext(ctx, q, s.runID, obj.NormalizedKey, obj.PlannedAction, obj.Checksum); err != nil {
			s.errf("audit insert_item: " + err.Error())
		}
	}
}

func (s *Subscriber) onObjectApplied(payload any) {
	ctx := context.Background()
	ev := payload.(*types.ObjectEvent)
	if _, err := s.conn.ExecContext(ctx, insertAttemptSQL,
		s.runID, 0, ev.NormalizedKey, ev.Checksum, "applied", "", time.Now().UTC()); err != nil {
		s.errf("audit insert_attempt: " + err.Error())
	}
}

func (s *Subscriber) onObjectSkipped(payload any) {
	ctx := context.Background()
	ev := payload.(*types.ObjectEvent)
	if _, err := s.conn.ExecContext(ctx, insertAttemptSQL,
		s.runID, 0, ev.NormalizedKey, ev.Checksum, "skipped", "", time.Now().UTC()); err != nil {
		s.errf("audit insert_attempt: " + err.Error())
	}
}

func (s *Subscriber) onObjectFailed(payload any) {
	ctx := context.Background()
	ev := payload.(*types.FailureEvent)
	if _, err := s.conn.ExecContext(ctx, insertAttemptSQL,
		s.runID, 0, ev.NormalizedKey, ev.Checksum, "failed", ev.Error, time.Now().UTC()); err != nil {
		s.errf("audit insert_attempt: " + err.Error())
	}
}

func (s *Subscriber) onRunFinished(payload any) {
	ctx := context.Background()
	ev := payload.(*types.RunFinished)
	if _, err := s.conn.ExecContext(ctx, updateRunSQL,
		time.Now().UTC(), ev.Result, ev.ExitCode, s.runID); err != nil {
		s.errf("audit update_run: " + err.Error())
	}
}
