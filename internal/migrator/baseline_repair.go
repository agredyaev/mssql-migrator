package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/planner"
)

func (r Runner) Baseline(ctx context.Context) error {
	if err := r.requireConfirmation(); err != nil {
		return err
	}
	baselineRunner := r
	baselineRunner.cfg.UpdatePolicy = config.UpdatePolicyNone
	session, err := r.startProtectedSession(ctx)
	if err != nil {
		return session.Fail(err, nil)
	}
	defer session.Close()
	conn := session.conn
	report := session.MigrationReport()
	if err := session.BootstrapMetadata(ctx); err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	successfulByKey, err := metadata.LoadSuccessfulChecksumsIfPresent(ctx, conn)
	if err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	layout, hash, err := session.ResolvePlanningLayout()
	if err != nil {
		return session.Fail(contracts.ErrInvalidInput, err)
	}
	session.SetLayoutHash(hash)
	plan, err := planner.BuildResolved(ctx, baselineRunner.cfg, successfulByKey, layout, hash, planner.SQLCatalogReader(conn))
	if err != nil {
		return session.Fail(contracts.ErrInvalidInput, err)
	}
	runID, err := baselineRunner.startRun(ctx, conn, "baseline", "", "", contracts.RollbackScope(baselineRunner.cfg.TransactionMode))
	if err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	recorder := newMetadataRecorder(baselineRunner.cfg, conn, runID)
	trackedObjectIDs, err := persistMigrationScope(ctx, conn, runID, plan)
	if err != nil {
		session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, contracts.ErrCriticalState, err))
		return session.Fail(contracts.ErrCriticalState, err)
	}
	plannedByKey := map[string]contracts.PlannedObject{}
	for _, object := range plan.Objects {
		plannedByKey[object.NormalizedKey] = object
		trackedObjectID := lookupTrackedObjectID(trackedObjectIDs, object.NormalizedKey)
		switch object.PlannedAction {
		case contracts.ActionSkipUnchanged, contracts.ActionAdoptExisting, contracts.ActionCreateObject:
			continue
		case contracts.ActionUpdateExistingModule, contracts.ActionUpdateExistingSupported, contracts.ActionReprocessChangedBlocked:
			failure := fmt.Errorf("%w: baseline found existing metadata drift for %s; use repair-checksum", contracts.ErrMetadataDrift, object.ObjectPath)
			if recordErr := recorder.recordObjectFailure(ctx, baselineFailureMetadataObject(object, trackedObjectID, baselineRunner.cfg.TransactionMode), failure, true); recordErr != nil {
				r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
			}
			session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, failure, nil))
			return session.Fail(failure, nil)
		default:
			failure := fmt.Errorf("%w: unsupported baseline object state %s for %s", contracts.ErrInvalidInput, object.PlannedAction, object.ObjectPath)
			if recordErr := recorder.recordObjectFailure(ctx, baselineFailureMetadataObject(object, trackedObjectID, baselineRunner.cfg.TransactionMode), failure, true); recordErr != nil {
				r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
			}
			session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, failure, nil))
			return session.Fail(failure, nil)
		}
	}
	if err := verifyBaselineCreatePermissions(ctx, conn, plan, layout); err != nil {
		var failure *baselinePreflightFailure
		if errors.As(err, &failure) {
			if strings.TrimSpace(failure.schemaName) != "" {
				if recordErr := recorder.recordSchemaFailure(ctx, failure.schemaName, err, true); recordErr != nil {
					r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
				}
			}
			if strings.TrimSpace(failure.object.Path) != "" {
				trackedObjectID := lookupTrackedObjectID(trackedObjectIDs, failure.object.NormalizedKey)
				planned := plannedByKey[failure.object.NormalizedKey]
				if planned.NormalizedKey == "" {
					planned = contracts.PlannedObject{ObjectPath: failure.object.Path, NormalizedKey: failure.object.NormalizedKey, Checksum: failure.object.Checksum}
				}
				if recordErr := recorder.recordObjectFailure(ctx, baselineFailureMetadataObject(planned, trackedObjectID, baselineRunner.cfg.TransactionMode), err, true); recordErr != nil {
					r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
				}
			}
		}
		session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, err, nil))
		return session.Fail(err, nil)
	}
	if err := baselineRunner.executePlanTracked(ctx, conn, layout, plan, report, runID, trackedObjectIDs); err != nil {
		if writeErr := session.WriteMigrationReport(); writeErr != nil {
			return contracts.Wrap(contracts.ErrCriticalState, writeErr)
		}
		session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, err, nil))
		return err
	}
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if err := recorder.recordRunResult(ctx, true, nil, nil); err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	return session.WriteMigrationReport()
}

func (r Runner) RepairChecksum(ctx context.Context) error {
	if err := r.requireConfirmation(); err != nil {
		return err
	}
	if strings.TrimSpace(r.cfg.RepairTarget) == "" {
		return fmt.Errorf("%w: --script is required", contracts.ErrInvalidInput)
	}
	session, err := r.startProtectedSession(ctx)
	if err != nil {
		return session.Fail(err, nil)
	}
	defer session.Close()
	conn := session.conn
	report := session.MigrationReport()
	if err := session.BootstrapMetadata(ctx); err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	successfulByKey, err := metadata.LoadSuccessfulChecksumsIfPresent(ctx, conn)
	if err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	layout, hash, err := session.ResolvePlanningLayout()
	if err != nil {
		return session.Fail(contracts.ErrInvalidInput, err)
	}
	session.SetLayoutHash(hash)
	target, err := resolveRepairObject(layout, r.cfg.RepairTarget)
	if err != nil {
		return session.Fail(contracts.ErrInvalidInput, err)
	}
	plan, err := planner.BuildResolved(ctx, r.cfg, successfulByKey, layout, hash, planner.SQLCatalogReader(conn))
	if err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	plannedTarget, err := resolveRepairPlanObject(plan, target.NormalizedKey)
	if err != nil {
		return session.Fail(contracts.ErrInvalidInput, err)
	}
	if err := validateRepairEligibility(target, plannedTarget); err != nil {
		return session.Fail(contracts.ErrInvalidInput, err)
	}
	catalog, err := planner.SQLCatalogReader(conn).ReadCatalogState(ctx)
	if err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	if _, exists := catalog.Objects[target.NormalizedKey]; !exists {
		return session.Fail(contracts.ErrInvalidInput, fmt.Errorf("repair target is missing from the database: %s", target.Path))
	}
	currentChecksum, exists := successfulByKey[target.NormalizedKey]
	if !exists {
		return session.Fail(contracts.ErrInvalidInput, fmt.Errorf("repair target has no successful metadata row: %s", target.Path))
	}
	runID, err := r.startRun(ctx, conn, "repair-checksum", "", "", contracts.RollbackScopeNone)
	if err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	recorder := newMetadataRecorder(r.cfg, conn, runID)
	trackedObjectID, err := persistRepairScope(ctx, conn, runID, target, plannedTarget, catalog, currentChecksum)
	if err != nil {
		session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, contracts.ErrCriticalState, err))
		return session.Fail(contracts.ErrCriticalState, err)
	}
	if currentChecksum == target.Checksum {
		report.Skipped = append(report.Skipped, contracts.ScriptResult{Script: target.Path, Type: target.Kind, Checksum: target.Checksum, Reason: "checksum_already_matches"})
		report.Result = "success"
		report.FinishedAt = time.Now().UTC()
		report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
		if err := recorder.recordRunResult(ctx, true, nil, nil); err != nil {
			return session.Fail(contracts.ErrCriticalState, err)
		}
		return session.WriteMigrationReport()
	}
	err = recorder.insertAttempt(ctx, metadata.AttemptRecord{
		TrackedObjectID:  trackedObjectID,
		ScriptName:       target.NormalizedKey,
		ScriptType:       contracts.ScriptTypeRepair,
		Checksum:         target.Checksum,
		Action:           contracts.ActionRepairChecksum,
		ExecutionMS:      0,
		Success:          true,
		TransactionMode:  config.TransactionModeNone,
		TransactionScope: config.TransactionModeNone,
		RollbackScope:    contracts.RollbackScopeNone,
		NoTransaction:    true,
		GitCommit:        r.cfg.GitCommit,
		GitBranch:        r.cfg.GitBranch,
		PipelineRunID:    r.cfg.PipelineRunID,
		PipelineURL:      logger.Redact(r.cfg.PipelineURL),
		AppliedBy:        r.cfg.Actor,
	})
	if err != nil {
		session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, contracts.ErrCriticalState, err))
		return session.Fail(contracts.ErrCriticalState, err)
	}
	err = recorder.updateTrackedObjectResult(ctx, target.NormalizedKey, true, "")
	if err != nil {
		session.WarnMetadataFinishFailure(recorder.recordRunResult(ctx, false, contracts.ErrCriticalState, err))
		return session.Fail(contracts.ErrCriticalState, err)
	}
	report.Applied = append(report.Applied, contracts.ScriptResult{Script: target.Path, Type: target.Kind, Checksum: target.Checksum})
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if err := recorder.recordRunResult(ctx, true, nil, nil); err != nil {
		return session.Fail(contracts.ErrCriticalState, err)
	}
	return session.WriteMigrationReport()
}

func resolveRepairObject(layout parser.Layout, selector string) (parser.Object, error) {
	selector = strings.TrimSpace(selector)
	normalizedSelector := parser.NormalizeTrackedName(selector)
	for _, object := range layout.Objects {
		if object.Path == selector || object.NormalizedKey == normalizedSelector || object.NormalizedKey == strings.ToLower(strings.TrimSpace(selector)) {
			return object, nil
		}
	}
	return parser.Object{}, fmt.Errorf("repair-checksum target not found in repo layout: %s", selector)
}

func persistRepairScope(ctx context.Context, conn *sql.Conn, runID string, object parser.Object, planned contracts.PlannedObject, catalog planner.CatalogState, currentChecksum string) (*int64, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin repair scope transaction: %w", err)
	}
	_, schemaExists := catalog.Schemas[object.NormalizedSchemaName]
	if err := metadata.InsertTrackedSchema(ctx, tx, metadata.TrackedSchemaRecord{
		RunID:                runID,
		SchemaName:           object.SchemaName,
		NormalizedSchemaName: object.NormalizedSchemaName,
		ExistsInDatabase:     boolPtr(schemaExists),
		Action:               contracts.SchemaActionExists,
		Success:              boolPtr(schemaExists),
	}); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	metadataMatch := currentChecksum == object.Checksum
	trackedObjectID, err := metadata.InsertTrackedObject(ctx, tx, metadata.TrackedObjectRecord{
		RunID:                runID,
		ObjectPath:           object.Path,
		SchemaName:           object.SchemaName,
		NormalizedSchemaName: object.NormalizedSchemaName,
		Kind:                 object.Kind,
		ObjectName:           object.ObjectName,
		NormalizedObjectName: object.NormalizedObjectName,
		ParentName:           object.ParentName,
		NormalizedParentName: object.NormalizedParentName,
		NormalizedKey:        object.NormalizedKey,
		Checksum:             object.Checksum,
		ExistsInDatabase:     boolPtr(true),
		MetadataMatch:        boolPtr(metadataMatch),
		PlannedAction:        planned.PlannedAction,
	})
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repair scope transaction: %w", err)
	}
	return &trackedObjectID, nil
}

func resolveRepairPlanObject(plan contracts.MigrationPlan, normalizedKey string) (contracts.PlannedObject, error) {
	for _, object := range plan.Objects {
		if object.NormalizedKey == normalizedKey {
			return object, nil
		}
	}
	return contracts.PlannedObject{}, fmt.Errorf("repair-checksum target not found in current plan: %s", normalizedKey)
}

func validateRepairEligibility(target parser.Object, planned contracts.PlannedObject) error {
	switch planned.PlannedAction {
	case contracts.ActionUpdateExistingModule, contracts.ActionUpdateExistingSupported, contracts.ActionReprocessChangedBlocked:
		return nil
	case contracts.ActionSkipUnchanged:
		return fmt.Errorf("repair-checksum is not needed for %s: the latest successful metadata checksum already matches the current repo SQL", target.Path)
	case contracts.ActionAdoptExisting:
		return fmt.Errorf("repair-checksum cannot run for %s: the object exists in the database but is not tracked yet, so use baseline or migrate to adopt it first", target.Path)
	case contracts.ActionCreateObject:
		return fmt.Errorf("repair-checksum cannot run for %s: the object is planned for creation and has no prior successful metadata checksum to repair", target.Path)
	default:
		return fmt.Errorf("repair-checksum cannot run for %s: planned action is %s and does not represent tracked checksum drift", target.Path, planned.PlannedAction)
	}
}
