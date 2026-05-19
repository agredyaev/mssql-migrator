package audit

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"sync"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/insert_history_openjson.sql
var insertHistoryOpenJSONSQL string

type Subscriber struct {
	conn          driver.Conn
	bootstrapErr  error
	notifier      types.ErrorNotifier
	bootstrapOnce sync.Once
	pending       []historyRecord
	pendingMu     sync.Mutex
}

func NewSubscriber(b bus.EventBus, conn driver.Conn) *Subscriber {
	s := &Subscriber{conn: conn}
	b.Subscribe(types.EventObjectApplied, s.onObjectApplied)
	b.Subscribe(types.EventObjectFailed, s.onObjectFailed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.notifier.SetErrorHandler(fn)
}

func (s *Subscriber) BootstrapError() error {
	return s.bootstrapErr
}

func (s *Subscriber) onObjectApplied(_ context.Context, payload any) {
	events, ok := bus.ParseObjectAppliedPayload(payload)
	if !ok {
		s.notifier.Notify("audit: unexpected payload type for EventObjectApplied")
		return
	}
	if len(events) == 0 {
		return
	}
	records := make([]historyRecord, len(events))
	for i := range events {
		records[i] = historyRecord{ev: events[i], event: "applied"}
	}
	s.enqueue(records)
}

func (s *Subscriber) onObjectFailed(_ context.Context, payload any) {
	failures, ok := bus.ParseObjectFailedPayload(payload)
	if !ok {
		s.notifier.Notify("audit: unexpected payload type for EventObjectFailed")
		return
	}
	if len(failures) == 0 {
		return
	}
	records := make([]historyRecord, len(failures))
	for i := range failures {
		records[i] = historyRecord{ev: &failures[i].ObjectEvent, event: "failed", errText: failures[i].Error}
	}
	s.enqueue(records)
}

func (s *Subscriber) onRunFinished(ctx context.Context, _ any) {
	s.flushPending(ctx)
}

func (s *Subscriber) enqueue(records []historyRecord) {
	s.pendingMu.Lock()
	s.pending = append(s.pending, records...)
	s.pendingMu.Unlock()
}

func (s *Subscriber) flushPending(ctx context.Context) {
	s.pendingMu.Lock()
	records := s.pending
	s.pending = nil
	s.pendingMu.Unlock()
	if len(records) == 0 {
		return
	}
	if err := s.boot(ctx); err != nil {
		return
	}
	s.insertHistoryBatch(ctx, records)
}

func (s *Subscriber) boot(ctx context.Context) error {
	s.bootstrapOnce.Do(func() {
		s.bootstrapErr = EnsureTables(ctx, s.conn)
	})
	if s.bootstrapErr != nil {
		s.notifier.Notify(fmt.Sprintf("audit bootstrap: %s", s.bootstrapErr.Error()))
	}
	return s.bootstrapErr
}

type historyRecord struct {
	ev      *types.ObjectEvent
	event   string
	errText string
}

func (s *Subscriber) insertHistoryBatch(ctx context.Context, records []historyRecord) {
	if len(records) == 0 {
		return
	}
	payload, err := marshalHistoryRecordsJSON(records)
	if err != nil {
		s.notifier.Notify(fmt.Sprintf("audit insert_history: %s", err.Error()))
		return
	}
	if _, err := s.conn.ExecContext(ctx, insertHistoryOpenJSONSQL, payload); err != nil {
		s.notifier.Notify(fmt.Sprintf("audit insert_history: %s", err.Error()))
		return
	}
	storeLatestChecksumsFromHistory(s.conn, records)
	bumpChecksumsCacheGeneration(s.conn)
}

var historyJSONPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 1024)
		return &b
	},
}

func marshalHistoryRecordsJSON(records []historyRecord) (string, error) {
	bufPtr := historyJSONPool.Get().(*[]byte)
	b := (*bufPtr)[:0]
	need := 2
	for _, rec := range records {
		need += len(rec.ev.NormalizedKey) + len(rec.ev.RecordKind) + len(rec.ev.Checksum) + len(rec.ev.GitHash) +
			len(rec.ev.GitAuthor) + len(rec.ev.GitDate) + len(rec.event) + len(rec.errText) + 128
	}
	if cap(b) < need {
		b = make([]byte, 0, need)
	}

	b = append(b, '[')
	for i, rec := range records {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '{')
		b = appendJSONStringField(b, "normalized_key", rec.ev.NormalizedKey, false)
		b = appendJSONStringField(b, "kind", rec.ev.RecordKind, true)
		b = appendJSONStringField(b, "checksum", rec.ev.Checksum, true)
		b = appendJSONStringField(b, "git_hash", rec.ev.GitHash, true)
		b = appendJSONStringField(b, "git_author", rec.ev.GitAuthor, true)
		b = appendJSONStringField(b, "git_date", rec.ev.GitDate, true)
		b = appendJSONStringField(b, "event", rec.event, true)
		if rec.errText != "" {
			b = appendJSONStringField(b, "error_text", rec.errText, true)
		}
		b = append(b, '}')
	}
	b = append(b, ']')

	out := string(b)
	*bufPtr = b[:0]
	historyJSONPool.Put(bufPtr)
	return out, nil
}

func appendJSONStringField(dst []byte, key, value string, withComma bool) []byte {
	if withComma {
		dst = append(dst, ',')
	}
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	dst = strconv.AppendQuote(dst, value)
	return dst
}
