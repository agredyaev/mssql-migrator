package audit

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/insert_history.sql
var insertHistorySQL string

//go:embed sql/insert_history_openjson.sql
var insertHistoryOpenJSONSQL string

type Subscriber struct {
	conn          driver.Conn
	notifier      types.ErrorNotifier
	bootstrapOnce sync.Once
	bootstrapErr  error
}

var insertHistoryOpenJSONSupport sync.Map

func NewSubscriber(b bus.EventBus, conn driver.Conn) *Subscriber {
	s := &Subscriber{conn: conn}
	b.Subscribe(types.EventObjectApplied, s.onObjectApplied)
	b.Subscribe(types.EventObjectFailed, s.onObjectFailed)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.notifier.SetErrorHandler(fn)
}

func (s *Subscriber) BootstrapError() error {
	return s.bootstrapErr
}

func (s *Subscriber) onObjectApplied(ctx context.Context, payload any) {
	if err := s.boot(ctx); err != nil {
		return
	}
	switch ev := payload.(type) {
	case *types.ObjectEvent:
		s.insertHistoryBatch(ctx, []historyRecord{{ev: ev, event: "applied"}})
	case []*types.ObjectEvent:
		if len(ev) == 0 {
			return
		}
		records := make([]historyRecord, len(ev))
		for i := range ev {
			records[i] = historyRecord{ev: ev[i], event: "applied"}
		}
		s.insertHistoryBatch(ctx, records)
	default:
		s.notifier.Notify("audit: unexpected payload type for EventObjectApplied")
	}
}

func (s *Subscriber) onObjectFailed(ctx context.Context, payload any) {
	if err := s.boot(ctx); err != nil {
		return
	}
	switch ev := payload.(type) {
	case *types.FailureEvent:
		s.insertHistoryBatch(ctx, []historyRecord{{ev: &ev.ObjectEvent, event: "failed", errText: ev.Error}})
	case []*types.FailureEvent:
		if len(ev) == 0 {
			return
		}
		records := make([]historyRecord, len(ev))
		for i := range ev {
			records[i] = historyRecord{ev: &ev[i].ObjectEvent, event: "failed", errText: ev[i].Error}
		}
		s.insertHistoryBatch(ctx, records)
	default:
		s.notifier.Notify("audit: unexpected payload type for EventObjectFailed")
	}
}

func (s *Subscriber) boot(ctx context.Context) error {
	s.bootstrapOnce.Do(func() {
		s.bootstrapErr = EnsureTables(ctx, s.conn)
	})
	if s.bootstrapErr != nil {
		s.notifier.Notify("audit bootstrap: " + s.bootstrapErr.Error())
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
	if useOpenJSONForHistoryInsert(s.conn) {
		ok, err := s.insertHistoryBatchOpenJSON(ctx, records)
		if ok {
			bumpChecksumsCacheGeneration(s.conn)
			return
		}
		if err != nil {
			s.notifier.Notify("audit insert_history: " + err.Error())
			return
		}
	}
	const colsPerRow = 8
	maxRows := driver.DefaultMaxParameters / colsPerRow
	if maxRows <= 0 {
		maxRows = 1
	}
	for start := 0; start < len(records); start += maxRows {
		end := start + maxRows
		if end > len(records) {
			end = len(records)
		}
		chunk := records[start:end]
		query, args := s.buildInsertHistoryBatchQuery(chunk)
		if _, err := s.conn.ExecContext(ctx, query, args...); err != nil {
			s.notifier.Notify("audit insert_history: " + err.Error())
			continue
		}
		bumpChecksumsCacheGeneration(s.conn)
	}
}

func (s *Subscriber) insertHistoryBatchOpenJSON(ctx context.Context, records []historyRecord) (bool, error) {
	payload, err := marshalHistoryRecordsJSON(records)
	if err != nil {
		return false, err
	}
	_, err = s.conn.ExecContext(ctx, insertHistoryOpenJSONSQL, payload)
	if err == nil {
		setOpenJSONHistoryInsertSupport(s.conn, true)
		return true, nil
	}
	if shouldFallbackOpenJSONHistoryInsert(s.conn, err) {
		setOpenJSONHistoryInsertSupport(s.conn, false)
		return false, nil
	}
	return false, err
}

func (s *Subscriber) buildInsertHistoryBatchQuery(records []historyRecord) (string, []any) {
	const prefix = "INSERT INTO azdo_deploy_meta.history (normalized_key, kind, checksum, git_hash, git_author, git_date, event, error_text, created_at)\nVALUES "
	const rowSQLLen = len("(@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, SYSUTCDATETIME())")
	var b strings.Builder
	b.Grow(len(prefix) + len(records)*(rowSQLLen+2))
	b.WriteString(prefix)

	args := make([]any, 0, len(records)*8)
	param := 1
	var scratch [20]byte
	for i, rec := range records {
		if i > 0 {
			b.WriteString(",\n")
		}
		gitDate, err := parseGitDate(rec.ev.GitDate)
		if err != nil {
			s.notifier.Notify("audit insert_history: bad git date: " + err.Error())
			gitDate = sqlDefaultDate
		}
		b.WriteByte('(')
		for col := 0; col < 8; col++ {
			if col > 0 {
				b.WriteString(", ")
			}
			b.WriteString("@p")
			n := strconv.AppendInt(scratch[:0], int64(param), 10)
			b.Write(n)
			param++
		}
		b.WriteString(", SYSUTCDATETIME())")

		args = append(args,
			rec.ev.NormalizedKey,
			rec.ev.RecordKind,
			rec.ev.Checksum,
			rec.ev.GitHash,
			rec.ev.GitAuthor,
			gitDate,
			rec.event,
			rec.errText,
		)
	}
	return b.String(), args
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

func useOpenJSONForHistoryInsert(conn driver.Conn) bool {
	key := historyInsertConnKey(conn)
	if v, ok := insertHistoryOpenJSONSupport.Load(key); ok {
		return v.(bool)
	}
	return strings.Contains(strings.ToLower(fmt.Sprintf("%T", conn)), "mssql")
}

func setOpenJSONHistoryInsertSupport(conn driver.Conn, enabled bool) {
	insertHistoryOpenJSONSupport.Store(historyInsertConnKey(conn), enabled)
}

func shouldFallbackOpenJSONHistoryInsert(conn driver.Conn, err error) bool {
	if _, known := insertHistoryOpenJSONSupport.Load(historyInsertConnKey(conn)); known {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "openjson") || strings.Contains(msg, "compatibility level")
}

func historyInsertConnKey(conn driver.Conn) string {
	v := reflect.ValueOf(conn)
	if !v.IsValid() {
		return "<nil>"
	}
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		return fmt.Sprintf("%T:%x", conn, v.Pointer())
	}
	return fmt.Sprintf("%T", conn)
}

var sqlDefaultDate = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

func parseGitDate(s string) (time.Time, error) {
	if s == "" {
		return sqlDefaultDate, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
