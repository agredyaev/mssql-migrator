package apply

import (
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/errors"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

//go:embed sql/create_schema.sql
var createSchemaSQL string

//go:embed sql/begin_transaction.sql
var beginTransactionSQL string

//go:embed sql/commit_transaction.sql
var commitTransactionSQL string

//go:embed sql/rollback.sql
var rollbackSQL string

const defaultBatchSize = 100

// allZeroChecksumHex is hex.EncodeToString([32]byte{}); avoids an allocation per
// ObjectEvent when the digest is unset (common in tests and checksum-less paths).
const allZeroChecksumHex = "0000000000000000000000000000000000000000000000000000000000000000"

type ApplyResult struct {
	Applied int
	Skipped int
	Failed  int
	Errors  []string
}

type Executor struct {
	BatchSize int
}

type batchedStmt struct {
	content       string
	normalizedKey string
	kind          string
	schemaName    string
	objectName    string
	sourceFile    string
	checksumSum   [32]byte // raw digest; hex materialized lazily for bus events
	checksumHex   string   // memo from checksumHexString (empty until first use)
	gitHash       string
	gitAuthor     string
	gitDate       string
	recordKind    string
}

func New() *Executor {
	return &Executor{BatchSize: defaultBatchSize}
}

func (e *Executor) Execute(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, layout fs.Layout, b bus.EventBus) (*ApplyResult, error) {
	if plan.Blocked {
		return nil, errors.ErrPlanBlocked
	}
	result := &ApplyResult{}

	for _, schema := range plan.Schemas {
		switch schema.Action {
		case types.SchemaActionCreateSchema:
			if strings.Contains(schema.SchemaName, "]") {
				return result, fmt.Errorf("invalid schema name: %q", schema.SchemaName)
			}
			q := strings.Replace(createSchemaSQL, "{{schema_name}}", schema.SchemaName, 1)
			if _, err := conn.ExecContext(ctx, q); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, err.Error())
				return result, nil
			}
			result.Applied++
			if b.HasHandlers(types.EventSchemaCreated) {
				b.Publish(ctx, types.EventSchemaCreated, &types.SchemaEvent{
					SchemaName: schema.SchemaName,
					Action:     types.SchemaActionCreateSchema,
				})
			}
		case types.SchemaActionExists:
			result.Skipped++
		}
	}

	objIndex := layout.ObjectsByPath()
	transIndex := layout.TransitionsByPath()

	txBatches, nonTxStmts := e.collectStatements(plan, objIndex, result)
	for _, batch := range txBatches {
		e.executeTxBatch(ctx, conn, batch, result, b)
	}
	hasApplied := b.HasHandlers(types.EventObjectApplied)
	hasFailed := b.HasHandlers(types.EventObjectFailed)
	var appliedEvents []*types.ObjectEvent
	var failedEvents []*types.FailureEvent
	if hasApplied {
		appliedEvents = make([]*types.ObjectEvent, 0, len(nonTxStmts))
	}
	if hasFailed {
		failedEvents = make([]*types.FailureEvent, 0, len(nonTxStmts))
	}
	for i := range nonTxStmts {
		applied, failed := e.executeNonTx(ctx, conn, &nonTxStmts[i], result, hasApplied, hasFailed)
		if applied != nil {
			appliedEvents = append(appliedEvents, applied)
		}
		if failed != nil {
			failedEvents = append(failedEvents, failed)
		}
	}
	if len(appliedEvents) > 0 {
		b.Publish(ctx, types.EventObjectApplied, appliedEvents)
	}
	if len(failedEvents) > 0 {
		b.Publish(ctx, types.EventObjectFailed, failedEvents)
	}

	e.executeTransitions(ctx, conn, plan, transIndex, result, b)
	if result.Applied > 0 {
		db.InvalidateInspectorCache(conn)
	}

	return result, nil
}

var kindOrder = map[string]int{
	"types":      0,
	"sequences":  1,
	"tables":     2,
	"synonyms":   3,
	"indexes":    4,
	"views":      5,
	"functions":  6,
	"procedures": 7,
	"triggers":   8,
}

// stmtCollectCandidate mirrors collectStatements gating before Content(): skip
// counters are applied in the main loop only.
func stmtCollectCandidate(obj types.PlannedObject, objIndex map[string]*fs.Object) bool {
	switch obj.PlannedAction {
	case types.ActionSkipUnchanged, types.ActionAdoptExisting:
		return false
	case types.ActionReprocessChanged:
		if len(obj.TransitionPaths) > 0 {
			return false
		}
		return false
	}
	return objIndex[obj.ObjectPath] != nil
}

func (e *Executor) collectStatements(plan types.MigrationPlan, objIndex map[string]*fs.Object, result *ApplyResult) ([][]batchedStmt, []batchedStmt) {
	batchSize := e.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	txHint, nonTxHint := 0, 0
	for _, obj := range plan.Objects {
		if !stmtCollectCandidate(obj, objIndex) {
			continue
		}
		if types.IsTransactionalKind(obj.Kind) {
			txHint++
		} else {
			nonTxHint++
		}
	}

	allTx := make([]batchedStmt, 0, txHint)
	nonTxStmts := make([]batchedStmt, 0, nonTxHint)

	for _, obj := range plan.Objects {
		switch obj.PlannedAction {
		case types.ActionSkipUnchanged, types.ActionAdoptExisting:
			result.Skipped++
			continue
		case types.ActionReprocessChanged:
			if len(obj.TransitionPaths) > 0 {
				continue
			}
			result.Skipped++
			continue
		}

		fsObj := objIndex[obj.ObjectPath]
		if fsObj == nil {
			continue
		}

		content, err := fsObj.Content()
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", obj.NormalizedKey, err.Error()))
			continue
		}

		stmt := batchedStmt{
			content:       content,
			normalizedKey: obj.NormalizedKey,
			kind:          obj.Kind,
			schemaName:    obj.SchemaName,
			objectName:    obj.ObjectName,
			sourceFile:    obj.ObjectPath,
			checksumSum:   obj.Checksum,
			recordKind:    "object",
		}
		stmt.gitHash, stmt.gitAuthor, stmt.gitDate = obj.GitStrings()

		if types.IsTransactionalKind(obj.Kind) {
			allTx = append(allTx, stmt)
		} else {
			nonTxStmts = append(nonTxStmts, stmt)
		}
	}

	if len(allTx) == 0 {
		return nil, nonTxStmts
	}

	numBatches := (len(allTx) + batchSize - 1) / batchSize
	txBatches := make([][]batchedStmt, 0, numBatches)
	for i := 0; i < len(allTx); {
		j := i + batchSize
		if j > len(allTx) {
			j = len(allTx)
		}
		batch := allTx[i:j]
		sort.Stable(kindSorter(batch))
		txBatches = append(txBatches, batch)
		i = j
	}

	return txBatches, nonTxStmts
}

type kindSorter []batchedStmt

func (k kindSorter) Len() int      { return len(k) }
func (k kindSorter) Swap(i, j int) { k[i], k[j] = k[j], k[i] }
func (k kindSorter) Less(i, j int) bool {
	oi, oki := kindOrder[k[i].kind]
	oj, okj := kindOrder[k[j].kind]
	if !oki {
		oi = 99
	}
	if !okj {
		oj = 99
	}
	return oi < oj
}

func (stmt *batchedStmt) checksumHexString() string {
	if stmt.checksumHex == "" {
		if stmt.checksumSum == ([32]byte{}) {
			stmt.checksumHex = allZeroChecksumHex
		} else {
			stmt.checksumHex = hex.EncodeToString(stmt.checksumSum[:])
		}
	}
	return stmt.checksumHex
}

func newObjectEvent(stmt *batchedStmt) *types.ObjectEvent {
	return &types.ObjectEvent{
		ObjectRef: types.ObjectRef{
			ObjectPath:    stmt.sourceFile,
			SchemaName:    stmt.schemaName,
			Kind:          stmt.kind,
			ObjectName:    stmt.objectName,
			NormalizedKey: stmt.normalizedKey,
		},
		Checksum: stmt.checksumHexString(),
		GitInfo: types.GitInfo{
			GitHash:   stmt.gitHash,
			GitAuthor: stmt.gitAuthor,
			GitDate:   stmt.gitDate,
		},
		RecordKind: stmt.recordKind,
	}
}

func newFailureEvent(stmt *batchedStmt, execErr string) *types.FailureEvent {
	return &types.FailureEvent{
		ObjectEvent: *newObjectEvent(stmt),
		Error:       execErr,
	}
}

func (e *Executor) executeTxBatch(ctx context.Context, conn driver.Conn, stmts []batchedStmt, result *ApplyResult, b bus.EventBus) {
	batchSQL := buildTxBatchSQL(stmts)
	_, err := conn.ExecContext(ctx, batchSQL)
	if err != nil {
		if rbErr := rollbackIfOpen(ctx, conn); rbErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("rollback: %s", rbErr.Error()))
		}
		hasFailed := b.HasHandlers(types.EventObjectFailed)
		hasApplied := b.HasHandlers(types.EventObjectApplied)
		for i := range stmts {
			stmt := &stmts[i]
			singleSQL := buildSingleTxSQL(stmt.content)
			if _, stmtErr := conn.ExecContext(ctx, singleSQL); stmtErr != nil {
				if rbErr := rollbackIfOpen(ctx, conn); rbErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("rollback: %s", rbErr.Error()))
				}
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", stmt.normalizedKey, stmtErr.Error()))
				if hasFailed {
					b.Publish(ctx, types.EventObjectFailed, newFailureEvent(stmt, stmtErr.Error()))
				}
			} else {
				result.Applied++
				if hasApplied {
					b.Publish(ctx, types.EventObjectApplied, newObjectEvent(stmt))
				}
			}
		}
		return
	}

	result.Applied += len(stmts)
	if b.HasHandlers(types.EventObjectApplied) {
		events := make([]*types.ObjectEvent, len(stmts))
		for i := range stmts {
			events[i] = newObjectEvent(&stmts[i])
		}
		b.Publish(ctx, types.EventObjectApplied, events)
	}
}

func (e *Executor) executeNonTx(ctx context.Context, conn driver.Conn, stmt *batchedStmt, result *ApplyResult, hasApplied, hasFailed bool) (*types.ObjectEvent, *types.FailureEvent) {
	if _, err := conn.ExecContext(ctx, stmt.content); err != nil {
		result.Failed++
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", stmt.normalizedKey, err.Error()))
		if hasFailed {
			return nil, newFailureEvent(stmt, err.Error())
		}
		return nil, nil
	}
	result.Applied++
	if hasApplied {
		return newObjectEvent(stmt), nil
	}
	return nil, nil
}

func (e *Executor) executeTransitions(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, transIndex map[string]*fs.TransitionScript, result *ApplyResult, b bus.EventBus) {
	hasFailed := b.HasHandlers(types.EventObjectFailed)
	hasApplied := b.HasHandlers(types.EventObjectApplied)
	for _, obj := range plan.Objects {
		if obj.PlannedAction != types.ActionReprocessChanged || len(obj.TransitionPaths) == 0 {
			continue
		}
		for _, tp := range obj.TransitionPaths {
			ts := transIndex[tp]
			if ts == nil {
				continue
			}
			content, err := ts.Content()
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", tp, err.Error()))
				if hasFailed {
					ms := newMigrationStmt(obj, tp, [32]byte{}, "", "", "")
					b.Publish(ctx, types.EventObjectFailed, newFailureEvent(&ms, err.Error()))
				}
				continue
			}

			gitHash, _ := ts.GitHash()
			gitAuthor, _ := ts.GitAuthor()
			gitDate, _ := ts.GitDate()
			cs, _ := ts.Checksum()

			sql := buildSingleTxSQL(content)
			if _, err := conn.ExecContext(ctx, sql); err != nil {
				if rbErr := rollbackIfOpen(ctx, conn); rbErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("rollback: %s", rbErr.Error()))
				}
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", tp, err.Error()))
				if hasFailed {
					ms := newMigrationStmt(obj, tp, cs, gitHash, gitAuthor, gitDate)
					b.Publish(ctx, types.EventObjectFailed, newFailureEvent(&ms, err.Error()))
				}
				continue
			}
			result.Applied++
			if hasApplied {
				ms := newMigrationStmt(obj, tp, cs, gitHash, gitAuthor, gitDate)
				b.Publish(ctx, types.EventObjectApplied, newObjectEvent(&ms))
			}
		}
	}
}

func newMigrationStmt(obj types.PlannedObject, tp string, checksum [32]byte, gitHash, gitAuthor, gitDate string) batchedStmt {
	return batchedStmt{
		normalizedKey: tp,
		kind:          obj.Kind,
		schemaName:    obj.SchemaName,
		objectName:    obj.ObjectName,
		sourceFile:    tp,
		checksumSum:   checksum,
		gitHash:       gitHash,
		gitAuthor:     gitAuthor,
		gitDate:       gitDate,
		recordKind:    "migration",
	}
}

var txSQLBuilderPool = sync.Pool{
	New: func() any { return new(strings.Builder) },
}

// buildSingleTxSQL wraps body in begin/commit once; uses a pooled Builder to
// avoid intermediate strings from repeated concatenation on failure paths.
func buildSingleTxSQL(body string) string {
	b := txSQLBuilderPool.Get().(*strings.Builder)
	defer txSQLBuilderPool.Put(b)
	b.Reset()
	b.Grow(len(beginTransactionSQL) + len(commitTransactionSQL) + len(body) + 2)
	b.WriteString(beginTransactionSQL)
	b.WriteByte('\n')
	b.WriteString(body)
	b.WriteByte('\n')
	b.WriteString(commitTransactionSQL)
	return b.String()
}

func buildTxBatchSQL(stmts []batchedStmt) string {
	n := len(beginTransactionSQL) + len(commitTransactionSQL) + len(stmts) + 1
	for _, stmt := range stmts {
		n += len(stmt.content)
	}
	b := txSQLBuilderPool.Get().(*strings.Builder)
	defer txSQLBuilderPool.Put(b)
	b.Reset()
	b.Grow(n)
	b.WriteString(beginTransactionSQL)
	b.WriteByte('\n')
	for i, stmt := range stmts {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(stmt.content)
	}
	b.WriteByte('\n')
	b.WriteString(commitTransactionSQL)
	return b.String()
}

func rollbackIfOpen(ctx context.Context, conn driver.Conn) error {
	_, err := conn.ExecContext(ctx, rollbackSQL)
	return err
}
