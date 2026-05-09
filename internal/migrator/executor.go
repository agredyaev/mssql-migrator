package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
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

func (r Runner) executePlanTracked(ctx context.Context, conn txConn, layout parser.Layout, plan contracts.MigrationPlan, report *contracts.MigrationReport, runID string, trackedObjectIDs map[string]int64) error {
	for _, schema := range plan.Schemas {
		if schema.Action == contracts.SchemaActionExists && runID != "" {
			metaCtx, cancel := metadataContext(ctx)
			err := metadata.UpdateTrackedSchemaResult(metaCtx, conn, runID, strings.ToLower(schema.SchemaName), true, "")
			cancel()
			if err != nil {
				report.Result = "failed"
				report.Failed = &contracts.Failure{Script: schema.SchemaName, Error: "critical metadata failure after schema discovery: " + logger.Redact(err.Error())}
				return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
			}
		}
		if schema.Action != contracts.SchemaActionCreateSchema {
			continue
		}
		statement := fmt.Sprintf("IF SCHEMA_ID('%s') IS NULL EXEC('CREATE SCHEMA [%s]')", escapeSQLString(schema.SchemaName), escapeSQLIdentifier(schema.SchemaName))
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			classifiedErr := classifySchemaExecutionError(schema.SchemaName, err)
			message := logger.Redact(classifiedErr.Error())
			if runID != "" {
				metaCtx, cancel := metadataContext(ctx)
				_ = metadata.UpdateTrackedSchemaResult(metaCtx, conn, runID, strings.ToLower(schema.SchemaName), false, message)
				cancel()
				metaCtx, cancel = metadataContext(ctx)
				_ = metadata.InsertAttempt(metaCtx, conn, metadata.AttemptRecord{
					RunID:         runID,
					ScriptName:    schema.SchemaName,
					ScriptType:    "schema",
					Checksum:      "-",
					Action:        contracts.ActionFail,
					ExecutionMS:   0,
					Success:       false,
					ErrorMessage:  message,
					GitCommit:     r.cfg.GitCommit,
					GitBranch:     r.cfg.GitBranch,
					PipelineRunID: r.cfg.PipelineRunID,
					PipelineURL:   logger.Redact(r.cfg.PipelineURL),
					AppliedBy:     r.cfg.Actor,
				})
				cancel()
			}
			report.Result = "failed"
			report.Failed = &contracts.Failure{Script: schema.SchemaName, Error: message}
			return classifiedErr
		}
		if runID != "" {
			metaCtx, cancel := metadataContext(ctx)
			err := metadata.UpdateTrackedSchemaResult(metaCtx, conn, runID, strings.ToLower(schema.SchemaName), true, "")
			cancel()
			if err != nil {
				report.Result = "failed"
				report.Failed = &contracts.Failure{Script: schema.SchemaName, Error: logger.Redact(err.Error())}
				return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
			}
			metaCtx, cancel = metadataContext(ctx)
			if err := metadata.InsertAttempt(metaCtx, conn, metadata.AttemptRecord{
				RunID:         runID,
				ScriptName:    schema.SchemaName,
				ScriptType:    "schema",
				Checksum:      "-",
				Action:        contracts.SchemaActionCreateSchema,
				ExecutionMS:   0,
				Success:       true,
				GitCommit:     r.cfg.GitCommit,
				GitBranch:     r.cfg.GitBranch,
				PipelineRunID: r.cfg.PipelineRunID,
				PipelineURL:   logger.Redact(r.cfg.PipelineURL),
				AppliedBy:     r.cfg.Actor,
			}); err != nil {
				cancel()
				report.Result = "failed"
				report.Failed = &contracts.Failure{Script: schema.SchemaName, Error: logger.Redact(err.Error())}
				return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
			}
			cancel()
		}
	}

	byKey := map[string]parser.Object{}
	for _, object := range layout.Objects {
		byKey[object.NormalizedKey] = object
	}
	for _, planned := range plannedObjectsInExecutionOrder(plan.Objects) {
		trackedObjectID := lookupTrackedObjectID(trackedObjectIDs, planned.NormalizedKey)
		if planned.PlannedAction != contracts.ActionCreateObject && planned.PlannedAction != contracts.ActionUpdateExistingModule && planned.PlannedAction != contracts.ActionUpdateExistingSupported {
			if planned.PlannedAction == contracts.ActionSkipUnchanged || planned.PlannedAction == contracts.ActionAdoptExisting {
				if err := r.recordPassiveObjectAction(ctx, conn, planned, runID, trackedObjectID); err != nil {
					report.Result = "failed"
					report.Failed = &contracts.Failure{Script: planned.ObjectPath, Error: logger.Redact(err.Error())}
					return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
				}
				report.Skipped = append(report.Skipped, contracts.ScriptResult{Script: planned.ObjectPath, Type: planned.Kind, Checksum: planned.Checksum, Reason: planned.PlannedAction})
				continue
			}
			report.Result = "failed"
			report.Failed = &contracts.Failure{Script: planned.ObjectPath, Error: "unsupported planned action: " + planned.PlannedAction}
			return fmt.Errorf("%w: unsupported planned action %s for %s", contracts.ErrInvalidInput, planned.PlannedAction, planned.ObjectPath)
		}
		object, ok := byKey[planned.NormalizedKey]
		if !ok {
			report.Result = "failed"
			report.Failed = &contracts.Failure{Script: planned.ObjectPath, Error: "object missing from verified layout"}
			return fmt.Errorf("%w: object missing from layout for %s", contracts.ErrInvalidInput, planned.NormalizedKey)
		}
		if err := r.applyObjectTracked(ctx, conn, object, planned, runID, trackedObjectID, report); err != nil {
			return err
		}
	}
	return nil
}

func (r Runner) recordAdoptedObject(ctx context.Context, execer metadata.Execer, planned contracts.PlannedObject) error {
	return r.recordPassiveObjectAction(ctx, execer, planned, "", nil)
}

func (r Runner) recordPassiveObjectAction(ctx context.Context, execer metadata.Execer, planned contracts.PlannedObject, runID string, trackedObjectID *int64) error {
	metaCtx, cancel := metadataContext(ctx)
	err := metadata.InsertAttempt(metaCtx, execer, metadata.AttemptRecord{
		RunID:            runID,
		TrackedObjectID:  trackedObjectID,
		ScriptName:       planned.NormalizedKey,
		ScriptType:       "object",
		Checksum:         planned.Checksum,
		Action:           planned.PlannedAction,
		ExecutionMS:      0,
		Success:          true,
		TransactionMode:  planned.TransactionMode,
		TransactionScope: planned.TransactionMode,
		RollbackScope:    planned.RollbackScope,
		NoTransaction:    planned.NoTransaction,
		GitCommit:        r.cfg.GitCommit,
		GitBranch:        r.cfg.GitBranch,
		PipelineRunID:    r.cfg.PipelineRunID,
		PipelineURL:      logger.Redact(r.cfg.PipelineURL),
		AppliedBy:        r.cfg.Actor,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("critical metadata failure after %s: database object state may drift from metadata: %w", planned.PlannedAction, err)
	}
	if runID != "" {
		metaCtx, cancel = metadataContext(ctx)
		err = metadata.UpdateTrackedObjectResult(metaCtx, execer, runID, planned.NormalizedKey, true, "")
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
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
		TransactionMode: contracts.TransactionModeForObject(r.cfg.TransactionMode, object.NoTransaction),
		RollbackScope:   contracts.RollbackScopeForObject(r.cfg.TransactionMode, object.NoTransaction),
		NoTransaction:   object.NoTransaction,
	}
	return r.applyObjectTracked(parent, conn, object, planned, "", nil, report)
}

func (r Runner) applyObjectTracked(parent context.Context, conn txConn, object parser.Object, planned contracts.PlannedObject, runID string, trackedObjectID *int64, report *contracts.MigrationReport) error {
	ctx, cancel := context.WithTimeout(parent, r.cfg.ScriptTimeout)
	defer cancel()
	startedAt := time.Now()
	stopProgress := r.startProgressLogger(ctx, object.Path, startedAt)
	executionErr := r.executeObject(ctx, conn, object)
	stopProgress()
	executionMS := int(time.Since(startedAt).Milliseconds())
	attempt := metadata.AttemptRecord{
		RunID:            runID,
		TrackedObjectID:  trackedObjectID,
		ScriptName:       object.NormalizedKey,
		ScriptType:       "object",
		Checksum:         object.Checksum,
		Action:           planned.PlannedAction,
		ExecutionMS:      executionMS,
		Success:          executionErr == nil,
		TransactionMode:  contracts.TransactionModeForObject(r.cfg.TransactionMode, object.NoTransaction),
		TransactionScope: contracts.TransactionModeForObject(r.cfg.TransactionMode, object.NoTransaction),
		RollbackScope:    contracts.RollbackScopeForObject(r.cfg.TransactionMode, object.NoTransaction),
		NoTransaction:    object.NoTransaction || r.cfg.TransactionMode == config.TransactionModeNone,
		GitCommit:        r.cfg.GitCommit,
		GitBranch:        r.cfg.GitBranch,
		PipelineRunID:    r.cfg.PipelineRunID,
		PipelineURL:      logger.Redact(r.cfg.PipelineURL),
		AppliedBy:        r.cfg.Actor,
	}
	if executionErr != nil {
		classifiedErr := classifyObjectExecutionError(object, planned, executionErr)
		attempt.Action = contracts.ActionFail
		attempt.ErrorMessage = logger.Redact(classifiedErr.Error())
		metaCtx, cancel := metadataContext(parent)
		if err := metadata.InsertAttempt(metaCtx, conn, attempt); err != nil {
			cancel()
			report.Result = "failed"
			report.Failed = &contracts.Failure{Script: object.Path, Error: "critical metadata failure after failed SQL: " + logger.Redact(err.Error())}
			return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
		}
		cancel()
		if runID != "" {
			metaCtx, cancel = metadataContext(parent)
			if err := metadata.UpdateTrackedObjectResult(metaCtx, conn, runID, object.NormalizedKey, false, attempt.ErrorMessage); err != nil {
				cancel()
				report.Result = "failed"
				report.Failed = &contracts.Failure{Script: object.Path, Error: "critical metadata failure after failed SQL: " + logger.Redact(err.Error())}
				return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
			}
			cancel()
		}
		report.Result = "failed"
		report.Failed = &contracts.Failure{Script: object.Path, Error: attempt.ErrorMessage}
		return classifiedErr
	}
	metaCtx, cancel := metadataContext(parent)
	if err := metadata.InsertAttempt(metaCtx, conn, attempt); err != nil {
		cancel()
		report.Result = "failed"
		report.Failed = &contracts.Failure{Script: object.Path, Error: "critical metadata failure after successful SQL: " + logger.Redact(err.Error())}
		return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
	}
	cancel()
	if runID != "" {
		metaCtx, cancel = metadataContext(parent)
		if err := metadata.UpdateTrackedObjectResult(metaCtx, conn, runID, object.NormalizedKey, true, ""); err != nil {
			cancel()
			report.Result = "failed"
			report.Failed = &contracts.Failure{Script: object.Path, Error: "critical metadata failure after successful SQL: " + logger.Redact(err.Error())}
			return fmt.Errorf("%w: %v", contracts.ErrCriticalState, err)
		}
		cancel()
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
	if object.NoTransaction || r.cfg.TransactionMode == config.TransactionModeNone {
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

func lookupTrackedObjectID(trackedObjectIDs map[string]int64, normalizedKey string) *int64 {
	if len(trackedObjectIDs) == 0 {
		return nil
	}
	id, ok := trackedObjectIDs[normalizedKey]
	if !ok {
		return nil
	}
	return &id
}

func plannedObjectsInExecutionOrder(items []contracts.PlannedObject) []contracts.PlannedObject {
	if len(items) < 2 {
		return items
	}
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
		for _, parentKey := range plannedParentCandidates(item) {
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

func plannedParentCandidates(item contracts.PlannedObject) []string {
	if strings.TrimSpace(item.ParentName) == "" {
		return nil
	}
	parentName := strings.ToLower(strings.TrimSpace(item.ParentName))
	schemaName := strings.ToLower(strings.TrimSpace(item.SchemaName))
	return []string{
		schemaName + "/tables/" + parentName,
		schemaName + "/views/" + parentName,
	}
}
