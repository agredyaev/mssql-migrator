package apply

import (
	"context"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/errors"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

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
	sourceFile    string
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
			q := fmt.Sprintf("CREATE SCHEMA [%s]", schema.SchemaName)
			if _, err := conn.ExecContext(ctx, q); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, err.Error())
				return result, nil
			}
			result.Applied++
		case types.SchemaActionExists:
			result.Skipped++
		}
	}

	batches := e.collectBatches(plan, layout, result)
	for _, batch := range batches {
		e.executeBatch(ctx, conn, batch, result)
	}

	return result, nil
}

func (e *Executor) collectBatches(plan types.MigrationPlan, layout fs.Layout, result *ApplyResult) [][]batchedStmt {
	var current []batchedStmt
	var batches [][]batchedStmt
	size := e.BatchSize
	if size <= 0 {
		size = defaultBatchSize
	}

	for _, obj := range plan.Objects {
		switch obj.PlannedAction {
		case types.ActionSkipUnchanged, types.ActionAdoptExisting:
			result.Skipped++
			continue
		}

		fsObj := lookupObject(layout, obj.SourceFile)
		if fsObj == nil {
			continue
		}

		content, err := fsObj.Content()
		if err != nil {
			continue
		}

		stmt := batchedStmt{
			content:       content,
			normalizedKey: obj.NormalizedKey,
			sourceFile:    obj.SourceFile,
		}
		current = append(current, stmt)

		if len(current) >= size {
			batches = append(batches, current)
			current = nil
		}
	}

	if len(current) > 0 {
		batches = append(batches, current)
	}

	return batches
}

func (e *Executor) executeBatch(ctx context.Context, conn driver.Conn, stmts []batchedStmt, result *ApplyResult) {
	batchSQL := buildBatchSQL(stmts)
	_, err := conn.ExecContext(ctx, batchSQL)
	if err != nil {
		for _, stmt := range stmts {
			if _, stmtErr := conn.ExecContext(ctx, stmt.content); stmtErr != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", stmt.normalizedKey, stmtErr.Error()))
			} else {
				result.Applied++
			}
		}
		return
	}

	result.Applied += len(stmts)
}

func buildBatchSQL(stmts []batchedStmt) string {
	var b strings.Builder
	for i, stmt := range stmts {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(stmt.content)
	}
	return b.String()
}

func lookupObject(layout fs.Layout, path string) *fs.Object {
	for _, obj := range layout.Objects {
		if obj.Path == path {
			return obj
		}
	}
	return nil
}
