package apply

import (
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"reporting-db-migrations/internal/bus"
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
	checksum      string
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
			b.Publish(ctx, types.EventSchemaCreated, &types.SchemaEvent{
				SchemaName: schema.SchemaName,
				Action:     types.SchemaActionCreateSchema,
			})
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
	for _, stmt := range nonTxStmts {
		e.executeNonTx(ctx, conn, stmt, result, b)
	}

	e.executeTransitions(ctx, conn, plan, transIndex, result, b)

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

func (e *Executor) collectStatements(plan types.MigrationPlan, objIndex map[string]*fs.Object, result *ApplyResult) ([][]batchedStmt, []batchedStmt) {
	var txCurrent []batchedStmt
	var txBatches [][]batchedStmt
	var nonTxStmts []batchedStmt

	batchSize := e.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

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
			checksum:      hex.EncodeToString(obj.Checksum[:]),
			recordKind:    "object",
		}
		stmt.gitHash, stmt.gitAuthor, stmt.gitDate = obj.GitStrings()

		if types.IsTransactionalKind(obj.Kind) {
			txCurrent = append(txCurrent, stmt)
			if len(txCurrent) >= batchSize {
				txBatches = append(txBatches, txCurrent)
				txCurrent = nil
			}
		} else {
			nonTxStmts = append(nonTxStmts, stmt)
		}
	}

	if len(txCurrent) > 0 {
		txBatches = append(txBatches, txCurrent)
	}

	for i := range txBatches {
		sort.Stable(kindSorter(txBatches[i]))
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

func newObjectEvent(stmt batchedStmt) *types.ObjectEvent {
	return &types.ObjectEvent{
		ObjectRef: types.ObjectRef{
			ObjectPath:    stmt.sourceFile,
			SchemaName:    stmt.schemaName,
			Kind:          stmt.kind,
			ObjectName:    stmt.objectName,
			NormalizedKey: stmt.normalizedKey,
		},
		Checksum: stmt.checksum,
		GitInfo: types.GitInfo{
			GitHash:   stmt.gitHash,
			GitAuthor: stmt.gitAuthor,
			GitDate:   stmt.gitDate,
		},
		RecordKind: stmt.recordKind,
	}
}

func newFailureEvent(stmt batchedStmt, execErr string) *types.FailureEvent {
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
		for _, stmt := range stmts {
			singleSQL := beginTransactionSQL + "\n" + stmt.content + "\n" + commitTransactionSQL
			if _, stmtErr := conn.ExecContext(ctx, singleSQL); stmtErr != nil {
				if rbErr := rollbackIfOpen(ctx, conn); rbErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("rollback: %s", rbErr.Error()))
				}
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", stmt.normalizedKey, stmtErr.Error()))
				b.Publish(ctx, types.EventObjectFailed, newFailureEvent(stmt, stmtErr.Error()))
			} else {
				result.Applied++
				b.Publish(ctx, types.EventObjectApplied, newObjectEvent(stmt))
			}
		}
		return
	}

	result.Applied += len(stmts)
	for _, stmt := range stmts {
		b.Publish(ctx, types.EventObjectApplied, newObjectEvent(stmt))
	}
}

func (e *Executor) executeNonTx(ctx context.Context, conn driver.Conn, stmt batchedStmt, result *ApplyResult, b bus.EventBus) {
	if _, err := conn.ExecContext(ctx, stmt.content); err != nil {
		result.Failed++
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", stmt.normalizedKey, err.Error()))
		b.Publish(ctx, types.EventObjectFailed, newFailureEvent(stmt, err.Error()))
		return
	}
	result.Applied++
	b.Publish(ctx, types.EventObjectApplied, newObjectEvent(stmt))
}

func (e *Executor) executeTransitions(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, transIndex map[string]*fs.TransitionScript, result *ApplyResult, b bus.EventBus) {
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
				b.Publish(ctx, types.EventObjectFailed, newFailureEvent(newMigrationStmt(obj, tp, "", "", "", ""), err.Error()))
				continue
			}

			gitHash, _ := ts.GitHash()
			gitAuthor, _ := ts.GitAuthor()
			gitDate, _ := ts.GitDate()
			cs, _ := ts.Checksum()

			sql := beginTransactionSQL + "\n" + content + "\n" + commitTransactionSQL
			if _, err := conn.ExecContext(ctx, sql); err != nil {
				if rbErr := rollbackIfOpen(ctx, conn); rbErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("rollback: %s", rbErr.Error()))
				}
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", tp, err.Error()))
				b.Publish(ctx, types.EventObjectFailed, newFailureEvent(newMigrationStmt(obj, tp, hex.EncodeToString(cs[:]), gitHash, gitAuthor, gitDate), err.Error()))
				continue
			}
			result.Applied++
			b.Publish(ctx, types.EventObjectApplied, newObjectEvent(newMigrationStmt(obj, tp, hex.EncodeToString(cs[:]), gitHash, gitAuthor, gitDate)))
		}
	}
}

func newMigrationStmt(obj types.PlannedObject, tp, checksum, gitHash, gitAuthor, gitDate string) batchedStmt {
	return batchedStmt{
		normalizedKey: tp,
		kind:          obj.Kind,
		schemaName:    obj.SchemaName,
		objectName:    obj.ObjectName,
		sourceFile:    tp,
		checksum:      checksum,
		gitHash:       gitHash,
		gitAuthor:     gitAuthor,
		gitDate:       gitDate,
		recordKind:    "migration",
	}
}

func buildTxBatchSQL(stmts []batchedStmt) string {
	var b strings.Builder
	b.WriteString(beginTransactionSQL)
	b.WriteString("\n")
	for i, stmt := range stmts {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(stmt.content)
	}
	b.WriteString("\n")
	b.WriteString(commitTransactionSQL)
	return b.String()
}

func rollbackIfOpen(ctx context.Context, conn driver.Conn) error {
	_, err := conn.ExecContext(ctx, rollbackSQL)
	return err
}
