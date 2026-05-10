package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
)

type txConn interface {
	metadata.Execer
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

func (r Runner) executePlan(ctx context.Context, conn txConn, layout parser.Layout, plan contracts.MigrationPlan, report *contracts.MigrationReport) error {
	return r.executePlanTracked(ctx, conn, layout, plan, report, "", nil)
}

func (r Runner) executePlanTracked(ctx context.Context, conn txConn, layout parser.Layout, plan contracts.MigrationPlan, report *contracts.MigrationReport, runID string, itemIDs map[string]int64) error {
	recorder := newMetadataRecorder(r.cfg, conn, nil, runID)
	for _, schema := range plan.Schemas {
		if err := r.executePlannedSchema(ctx, conn, recorder, schema, runID, report); err != nil {
			return err
		}
	}

	byKey := map[string]parser.Object{}
	for _, object := range layout.Objects {
		byKey[object.NormalizedKey] = object
	}
	for _, planned := range plannedObjectsInExecutionOrder(plan.Objects) {
		if err := r.executePlannedObject(ctx, conn, byKey, planned, runID, itemIDs, report); err != nil {
			return err
		}
	}
	return nil
}

func (r Runner) executePlannedSchema(ctx context.Context, conn txConn, recorder metadataRecorder, schema contracts.PlannedSchema, runID string, report *contracts.MigrationReport) error {
	if schema.Action == contracts.SchemaActionExists && runID != "" {
		err := recorder.attempt.SchemaSuccess(ctx, schema.SchemaName, schema.Action, false)
		if err != nil {
			setCriticalMetadataFailure(report, schema.SchemaName, "critical metadata failure after schema discovery: ", err)
			return contracts.Wrap(contracts.ErrCriticalState, err)
		}
	}
	if schema.Action != contracts.SchemaActionCreateSchema {
		return nil
	}
	statement := fmt.Sprintf("IF SCHEMA_ID('%s') IS NULL EXEC('CREATE SCHEMA [%s]')", escapeSQLString(schema.SchemaName), escapeSQLIdentifier(schema.SchemaName))
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		classifiedErr := classifySchemaExecutionError(schema.SchemaName, err)
		if runID != "" {
			_ = recorder.attempt.SchemaFailure(ctx, schema.SchemaName, classifiedErr, true)
		}
		setFailedResult(report, schema.SchemaName, classifiedErr.Error())
		return classifiedErr
	}
	if runID == "" {
		return nil
	}
	if err := recorder.attempt.SchemaSuccess(ctx, schema.SchemaName, contracts.SchemaActionCreateSchema, true); err != nil {
		setFailedResult(report, schema.SchemaName, err.Error())
		return contracts.Wrap(contracts.ErrCriticalState, err)
	}
	return nil
}

func (r Runner) executePlannedObject(ctx context.Context, conn txConn, byKey map[string]parser.Object, planned contracts.PlannedObject, runID string, itemIDs map[string]int64, report *contracts.MigrationReport) error {
	itemID := lookupItemID(itemIDs, planned.NormalizedKey)
	if planned.PlannedAction == contracts.ActionSkipUnchanged || planned.PlannedAction == contracts.ActionAdoptExisting {
		if err := r.recordPassiveObjectAction(ctx, conn, planned, runID, itemID); err != nil {
			setFailedResult(report, planned.ObjectPath, err.Error())
			return contracts.Wrap(contracts.ErrCriticalState, err)
		}
		report.Skipped = append(report.Skipped, contracts.ScriptResult{Script: planned.ObjectPath, Type: planned.Kind, Checksum: planned.Checksum, Reason: planned.PlannedAction})
		return nil
	}
	if planned.PlannedAction != contracts.ActionCreateObject && planned.PlannedAction != contracts.ActionUpdateExistingModule && planned.PlannedAction != contracts.ActionUpdateExistingSupported {
		setFailedResult(report, planned.ObjectPath, "unsupported planned action: "+planned.PlannedAction)
		return fmt.Errorf("%w: unsupported planned action %s for %s", contracts.ErrInvalidInput, planned.PlannedAction, planned.ObjectPath)
	}
	object, ok := byKey[planned.NormalizedKey]
	if !ok {
		setFailedResult(report, planned.ObjectPath, "object missing from verified layout")
		return fmt.Errorf("%w: object missing from layout for %s", contracts.ErrInvalidInput, planned.NormalizedKey)
	}
	return r.applyObjectTracked(ctx, conn, object, planned, runID, itemID, report)
}

func (r Runner) recordAdoptedObject(ctx context.Context, execer metadata.Execer, planned contracts.PlannedObject) error {
	return r.recordPassiveObjectAction(ctx, execer, planned, "", nil)
}

func (r Runner) recordPassiveObjectAction(ctx context.Context, execer metadata.Execer, planned contracts.PlannedObject, runID string, itemID *int64) error {
	recorder := newMetadataRecorder(r.cfg, execer, nil, runID)
	return recorder.attempt.ObjectSuccess(ctx, passiveMetadataObject(planned, itemID), true)
}

func (r Runner) applyObject(parent context.Context, conn txConn, object parser.Object, report *contracts.MigrationReport) error {
	planned := contracts.PlannedObject{
		ObjectPath:      object.Path,
		SchemaName:      object.SchemaName,
		Kind:            object.Kind,
		ObjectName:      object.ObjectName,
		ParentName:      object.ParentName,
		NormalizedKey:   object.NormalizedKey,
		Checksum:        object.Checksum,
		PlannedAction:   contracts.ActionCreateObject,
		TransactionMode: transactionModeForObject(r.cfg.TransactionMode, object.NoTransaction),
		RollbackScope:   rollbackScopeForObject(r.cfg.TransactionMode, object.NoTransaction),
		NoTransaction:   noTransactionForObject(r.cfg.TransactionMode, object.NoTransaction),
	}
	return r.applyObjectTracked(parent, conn, object, planned, "", nil, report)
}

func (r Runner) applyObjectTracked(parent context.Context, conn txConn, object parser.Object, planned contracts.PlannedObject, runID string, itemID *int64, report *contracts.MigrationReport) error {
	ctx, cancel := context.WithTimeout(parent, r.cfg.ScriptTimeout)
	defer cancel()
	startedAt := time.Now()
	stopProgress := r.startProgressLogger(ctx, object.Path, startedAt)
	executionErr := r.executeObject(ctx, conn, object)
	stopProgress()
	executionMS := int(time.Since(startedAt).Milliseconds())
	recorder := newMetadataRecorder(r.cfg, conn, nil, runID)
	recordedObject := executedMetadataObject(object, planned, itemID, executionMS)
	if executionErr != nil {
		classifiedErr := classifyObjectExecutionError(object, planned, executionErr)
		if err := recorder.attempt.ObjectFailure(parent, recordedObject, classifiedErr, true); err != nil {
			setCriticalMetadataFailure(report, object.Path, "critical metadata failure after failed SQL: ", err)
			return contracts.Wrap(contracts.ErrCriticalState, err)
		}
		setFailedResult(report, object.Path, classifiedErr.Error())
		return classifiedErr
	}
	if err := recorder.attempt.ObjectSuccess(parent, recordedObject, true); err != nil {
		setCriticalMetadataFailure(report, object.Path, "critical metadata failure after successful SQL: ", err)
		return contracts.Wrap(contracts.ErrCriticalState, err)
	}
	report.Applied = append(report.Applied, contracts.ScriptResult{Script: object.Path, Type: object.Kind, Checksum: object.Checksum, ExecutionMS: executionMS})
	r.log.Info("object_applied", fmt.Sprintf("object=%s execution_ms=%d", object.Path, executionMS))
	return nil
}

func (r Runner) executeObject(ctx context.Context, conn txConn, object parser.Object) error {
	batches, err := parser.SplitGO(object.Content)
	if err != nil {
		return err
	}
	if noTransactionForObject(r.cfg.TransactionMode, object.NoTransaction) {
		return executeBatches(ctx, conn, batches)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := executeBatches(ctx, tx, batches); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("execute batch: %w; rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func executeBatches(ctx context.Context, execer metadata.Execer, batches []parser.Batch) error {
	for _, batch := range batches {
		for i := 0; i < batch.Repeat; i++ {
			if _, err := execer.ExecContext(ctx, batch.SQL); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r Runner) startProgressLogger(ctx context.Context, scriptName string, startedAt time.Time) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.log.Info("script_running", fmt.Sprintf("script=%s elapsed=%s", scriptName, time.Since(startedAt).Round(time.Second)))
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() {
		once.Do(func() { close(done) })
		<-stopped
	}
}

func escapeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func escapeSQLIdentifier(value string) string {
	return strings.ReplaceAll(value, "]", "]]")
}

func lookupItemID(itemIDs map[string]int64, normalizedKey string) *int64 {
	if len(itemIDs) == 0 {
		return nil
	}
	id, ok := itemIDs[normalizedKey]
	if !ok {
		return nil
	}
	return &id
}

func plannedObjectsInExecutionOrder(items []contracts.PlannedObject) []contracts.PlannedObject {
	if len(items) < 2 {
		return items
	}
	resolver := objectDependencyResolver{}
	original := append([]contracts.PlannedObject(nil), items...)
	byKey := make(map[string]contracts.PlannedObject, len(original))
	for _, item := range original {
		byKey[item.NormalizedKey] = item
	}
	ordered := make([]contracts.PlannedObject, 0, len(original))
	visiting := map[string]bool{}
	seen := map[string]bool{}
	var visit func(contracts.PlannedObject)
	visit = func(item contracts.PlannedObject) {
		if item.NormalizedKey == "" || seen[item.NormalizedKey] {
			return
		}
		if visiting[item.NormalizedKey] {
			return
		}
		visiting[item.NormalizedKey] = true
		for _, parentKey := range resolver.ParentCandidates(item.SchemaName, item.ParentName) {
			if parent, ok := byKey[parentKey]; ok {
				visit(parent)
				break
			}
		}
		delete(visiting, item.NormalizedKey)
		if seen[item.NormalizedKey] {
			return
		}
		seen[item.NormalizedKey] = true
		ordered = append(ordered, item)
	}
	for _, item := range original {
		visit(item)
	}
	return ordered
}
