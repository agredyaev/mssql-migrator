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
	errf          func(msg string)
	bootstrapOnce sync.Once
}

func NewSubscriber(b bus.EventBus, conn driver.Conn) *Subscriber {
	s := &Subscriber{conn: conn}
	s.errf = func(msg string) {}
	b.Subscribe(types.EventObjectApplied, s.onObjectApplied)
	b.Subscribe(types.EventObjectFailed, s.onObjectFailed)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.errf = fn
}

func (s *Subscriber) onObjectApplied(payload any) {
	ctx := context.Background()
	ev := payload.(*types.ObjectEvent)
	s.bootOnce(ctx)
	s.insertHistory(ctx, ev, "applied", "")
}

func (s *Subscriber) onObjectFailed(payload any) {
	ctx := context.Background()
	ev := payload.(*types.FailureEvent)
	s.bootOnce(ctx)
	s.insertHistory(ctx, &ev.ObjectEvent, "failed", ev.Error)
}

func (s *Subscriber) bootOnce(ctx context.Context) {
	s.bootstrapOnce.Do(func() {
		if err := EnsureTables(ctx, s.conn); err != nil {
			s.errf("audit bootstrap: " + err.Error())
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
		s.errf("audit insert_history: " + err.Error())
	}
}

func parseGitDate(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse("2006-01-02T15:04:05-07:00", s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z", s)
		if err != nil {
			return time.Now().UTC()
		}
	}
	return t
}
