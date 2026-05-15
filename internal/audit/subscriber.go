package audit

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/insert_history.sql
var insertHistorySQL string

type Subscriber struct {
	conn          driver.Conn
	notifier      types.ErrorNotifier
	bootstrapOnce sync.Once
}

func NewSubscriber(b bus.EventBus, conn driver.Conn) *Subscriber {
	s := &Subscriber{conn: conn}
	b.Subscribe(types.EventObjectApplied, s.onObjectApplied)
	b.Subscribe(types.EventObjectFailed, s.onObjectFailed)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.notifier.SetErrorHandler(fn)
}

func (s *Subscriber) onObjectApplied(ctx context.Context, payload any) {
	ev, ok := payload.(*types.ObjectEvent)
	if !ok {
		s.notifier.Notify("audit: unexpected payload type for EventObjectApplied")
		return
	}
	s.bootOnce(ctx)
	s.insertHistory(ctx, ev, "applied", "")
}

func (s *Subscriber) onObjectFailed(ctx context.Context, payload any) {
	ev, ok := payload.(*types.FailureEvent)
	if !ok {
		s.notifier.Notify("audit: unexpected payload type for EventObjectFailed")
		return
	}
	s.bootOnce(ctx)
	s.insertHistory(ctx, &ev.ObjectEvent, "failed", ev.Error)
}

func (s *Subscriber) bootOnce(ctx context.Context) {
	s.bootstrapOnce.Do(func() {
		if err := EnsureTables(ctx, s.conn); err != nil {
			s.notifier.Notify("audit bootstrap: " + err.Error())
		}
	})
}

func (s *Subscriber) insertHistory(ctx context.Context, ev *types.ObjectEvent, event, errText string) {
	gitDate := parseGitDate(ev.GitDate)
	if _, err := s.conn.ExecContext(ctx, insertHistorySQL,
		ev.NormalizedKey,
		ev.RecordKind,
		ev.Checksum,
		ev.GitHash,
		ev.GitAuthor,
		gitDate,
		event,
		errText,
	); err != nil {
		s.notifier.Notify("audit insert_history: " + err.Error())
	}
}

func parseGitDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
