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
	"reporting-db-migrations/internal/reports"
)

func (r Runner) Baseline(ctx context.Context) error {
	if err := r.requireConfirmation(); err != nil {
		return err
	}
	baselineRunner := r
	baselineRunner.cfg.UpdatePolicy = config.UpdatePolicyNone
	report, conn, closeFn, err := r.prepareProtectedRun(ctx)
	if err != nil {
		return r.writeFailedMigration(report, err, nil)
	}
	defer closeFn()
	if err := r.acquireLock(ctx, conn); err != nil {
		return r.writeFailedMigration(report, err, nil)
	}
	if err := bootstrapMetadata(ctx, conn); err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	successfulByKey, err := metadata.LoadSuccessfulChecksumsIfPresent(ctx, conn)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	layout, hash, err := planner.ResolvePlanningLayoutForRunner(r.cfg)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	report.LayoutHash = hash
	plan, err := planner.BuildResolved(ctx, baselineRunner.cfg, successfulByKey, layout, hash, planner.SQLCatalogReader(conn))
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	runID, err := baselineRunner.startRun(ctx, conn, "baseline", "", "", rollbackScope(baselineRunner.cfg.TransactionMode))
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	trackedObjectIDs, err := persistMigrationScope(ctx, conn, runID, plan)
	if err != nil {
		_ = finishRun(ctx, conn, runID, false, contracts.ErrCriticalState, err)
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	plannedByKey := map[string]contracts.PlannedObject{}
	for _, object := range plan.Objects {
		plannedByKey[object.NormalizedKey] = object
		trackedObjectID := lookupTrackedObjectID(trackedObjectIDs, object.NormalizedKey)
		switch object.PlannedAction {
		case "skip_unchanged", "adopt_existing", "create_object":
			continue
		case "update_existing_module", "update_existing_supported", "reprocess_changed_blocked":
			failure := fmt.Errorf("%w: baseline found existing metadata drift for %s; use repair-checksum", contracts.ErrChecksumMismatch, object.ObjectPath)
			_ = recordBaselineFailure(ctx, conn, runID, object, trackedObjectID, failure, baselineRunner.cfg)
			_ = finishRun(ctx, conn, runID, false, failure, nil)
			return r.writeFailedMigration(report, failure, nil)
		default:
			failure := fmt.Errorf("%w: unsupported baseline object state %s for %s", contracts.ErrInvalidInput, object.PlannedAction, object.ObjectPath)
			_ = recordBaselineFailure(ctx, conn, runID, object, trackedObjectID, failure, baselineRunner.cfg)
			_ = finishRun(ctx, conn, runID, false, failure, nil)
			return r.writeFailedMigration(report, failure, nil)
		}
	}
	if err := verifyBaselineCreatePermissions(ctx, conn, plan, layout); err != nil {
		var failure *baselinePreflightFailure
		if errors.As(err, &failure) {
			if strings.TrimSpace(failure.schemaName) != "" {
				_ = recordBaselineSchemaFailure(ctx, conn, runID, failure.schemaName, err, baselineRunner.cfg)
			}
			if strings.TrimSpace(failure.object.Path) != "" {
				trackedObjectID := lookupTrackedObjectID(trackedObjectIDs, failure.object.NormalizedKey)
				planned := plannedByKey[failure.object.NormalizedKey]
				if planned.NormalizedKey == "" {
					planned = contracts.PlannedObject{ObjectPath: failure.object.Path, NormalizedKey: failure.object.NormalizedKey, Checksum: failure.object.Checksum}
				}
				_ = recordBaselineFailure(ctx, conn, runID, planned, trackedObjectID, err, baselineRunner.cfg)
			}
		}
		_ = finishRun(ctx, conn, runID, false, err, nil)
		return r.writeFailedMigration(report, err, nil)
	}
	if err := baselineRunner.executePlanTracked(ctx, conn, layout, plan, &report, runID, trackedObjectIDs); err != nil {
		if writeErr := reports.WriteMigration(r.cfg.ReportDir, report); writeErr != nil {
			return fmt.Errorf("%w: %v", contracts.ErrCriticalState, writeErr)
		}
		_ = finishRun(ctx, conn, runID, false, err, nil)
		return err
	}
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if err := finishRun(ctx, conn, runID, true, nil, nil); err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	return reports.WriteMigration(r.cfg.ReportDir, report)
}

func (r Runner) RepairChecksum(ctx context.Context) error {
	if err := r.requireConfirmation(); err != nil {
		return err
	}
	if strings.TrimSpace(r.cfg.RepairTarget) == "" {
		return fmt.Errorf("%w: --script is required", contracts.ErrInvalidInput)
	}
	report, conn, closeFn, err := r.prepareProtectedRun(ctx)
	if err != nil {
		return r.writeFailedMigration(report, err, nil)
	}
	defer closeFn()
	if err := r.acquireLock(ctx, conn); err != nil {
		return r.writeFailedMigration(report, err, nil)
	}
	if err := bootstrapMetadata(ctx, conn); err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	successfulByKey, err := metadata.LoadSuccessfulChecksumsIfPresent(ctx, conn)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	layout, hash, err := planner.ResolvePlanningLayoutForRunner(r.cfg)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	report.LayoutHash = hash
	target, err := resolveRepairObject(layout, r.cfg.RepairTarget)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, err)
	}
	catalog, err := planner.SQLCatalogReader(conn).ReadCatalogState(ctx)
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	if _, exists := catalog.Objects[target.NormalizedKey]; !exists {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, fmt.Errorf("repair target is missing from the database: %s", target.Path))
	}
	currentChecksum, exists := successfulByKey[target.NormalizedKey]
	if !exists {
		return r.writeFailedMigration(report, contracts.ErrInvalidInput, fmt.Errorf("repair target has no successful metadata row: %s", target.Path))
	}
	runID, err := r.startRun(ctx, conn, "repair-checksum", "", "", "none")
	if err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	trackedObjectID, err := persistRepairScope(ctx, conn, runID, target, catalog, currentChecksum)
	if err != nil {
		_ = finishRun(ctx, conn, runID, false, contracts.ErrCriticalState, err)
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	if currentChecksum == target.Checksum {
		report.Skipped = append(report.Skipped, contracts.ScriptResult{Script: target.Path, Type: target.Kind, Checksum: target.Checksum, Reason: "checksum_already_matches"})
		report.Result = "success"
		report.FinishedAt = time.Now().UTC()
		report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
		if err := finishRun(ctx, conn, runID, true, nil, nil); err != nil {
			return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
		}
		return reports.WriteMigration(r.cfg.ReportDir, report)
	}
	if err := metadata.InsertAttempt(ctx, conn, metadata.AttemptRecord{
		RunID:            runID,
		TrackedObjectID:  trackedObjectID,
		ScriptName:       target.NormalizedKey,
		ScriptType:       "repair",
		Checksum:         target.Checksum,
		Action:           "repair_checksum",
		ExecutionMS:      0,
		Success:          true,
		TransactionMode:  "none",
		TransactionScope: "none",
		RollbackScope:    "none",
		NoTransaction:    true,
		GitCommit:        r.cfg.GitCommit,
		GitBranch:        r.cfg.GitBranch,
		PipelineRunID:    r.cfg.PipelineRunID,
		PipelineURL:      logger.Redact(r.cfg.PipelineURL),
		AppliedBy:        r.cfg.Actor,
	}); err != nil {
		_ = finishRun(ctx, conn, runID, false, contracts.ErrCriticalState, err)
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	if err := metadata.UpdateTrackedObjectResult(ctx, conn, runID, target.NormalizedKey, true, ""); err != nil {
		_ = finishRun(ctx, conn, runID, false, contracts.ErrCriticalState, err)
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	report.Applied = append(report.Applied, contracts.ScriptResult{Script: target.Path, Type: target.Kind, Checksum: target.Checksum})
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if err := finishRun(ctx, conn, runID, true, nil, nil); err != nil {
		return r.writeFailedMigration(report, contracts.ErrCriticalState, err)
	}
	return reports.WriteMigration(r.cfg.ReportDir, report)
}

func recordBaselineFailure(ctx context.Context, execer metadata.Execer, runID string, object contracts.PlannedObject, trackedObjectID *int64, failure error, cfg config.Config) error {
	message := logger.Redact(failure.Error())
	if err := metadata.UpdateTrackedObjectResult(ctx, execer, runID, object.NormalizedKey, false, message); err != nil {
		return err
	}
	return metadata.InsertAttempt(ctx, execer, metadata.AttemptRecord{
		RunID:            runID,
		TrackedObjectID:  trackedObjectID,
		ScriptName:       object.NormalizedKey,
		ScriptType:       "object",
		Checksum:         object.Checksum,
		Action:           "fail",
		ExecutionMS:      0,
		Success:          false,
		ErrorMessage:     message,
		TransactionMode:  transactionModeForObject(cfg.TransactionMode, false),
		TransactionScope: transactionModeForObject(cfg.TransactionMode, false),
		RollbackScope:    rollbackScope(cfg.TransactionMode),
		NoTransaction:    cfg.TransactionMode == config.TransactionModeNone,
		GitCommit:        cfg.GitCommit,
		GitBranch:        cfg.GitBranch,
		PipelineRunID:    cfg.PipelineRunID,
		PipelineURL:      logger.Redact(cfg.PipelineURL),
		AppliedBy:        cfg.Actor,
	})
}

func recordBaselineSchemaFailure(ctx context.Context, execer metadata.Execer, runID string, schemaName string, failure error, cfg config.Config) error {
	message := logger.Redact(failure.Error())
	normalizedSchemaName := strings.ToLower(strings.TrimSpace(schemaName))
	if err := metadata.UpdateTrackedSchemaResult(ctx, execer, runID, normalizedSchemaName, false, message); err != nil {
		return err
	}
	return metadata.InsertAttempt(ctx, execer, metadata.AttemptRecord{
		RunID:         runID,
		ScriptName:    schemaName,
		ScriptType:    "schema",
		Checksum:      "-",
		Action:        "fail",
		ExecutionMS:   0,
		Success:       false,
		ErrorMessage:  message,
		GitCommit:     cfg.GitCommit,
		GitBranch:     cfg.GitBranch,
		PipelineRunID: cfg.PipelineRunID,
		PipelineURL:   logger.Redact(cfg.PipelineURL),
		AppliedBy:     cfg.Actor,
	})
}

func resolveRepairObject(layout parser.Layout, selector string) (parser.Object, error) {
	selector = strings.TrimSpace(selector)
	normalizedSelector := parser.NormalizeTrackedName(selector)
	for _, object := range layout.Objects {
		if object.Path == selector || object.NormalizedKey == normalizedSelector || object.NormalizedKey == strings.ToLower(strings.TrimSpace(selector)) {
			return object, nil
		}
	}
	return parser.Object{}, fmt.Errorf("repair target not found in repo layout: %s", selector)
}

func persistRepairScope(ctx context.Context, conn *sql.Conn, runID string, object parser.Object, catalog planner.CatalogState, currentChecksum string) (*int64, error) {
	_, schemaExists := catalog.Schemas[object.NormalizedSchemaName]
	if err := metadata.InsertTrackedSchema(ctx, conn, metadata.TrackedSchemaRecord{
		RunID:                runID,
		SchemaName:           object.SchemaName,
		NormalizedSchemaName: object.NormalizedSchemaName,
		ExistsInDatabase:     boolPtr(schemaExists),
		Action:               "exists",
		Success:              boolPtr(schemaExists),
	}); err != nil {
		return nil, err
	}
	metadataMatch := currentChecksum == object.Checksum
	trackedObjectID, err := metadata.InsertTrackedObject(ctx, conn, metadata.TrackedObjectRecord{
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
		PlannedAction:        "skip_unchanged",
	})
	if err != nil {
		return nil, err
	}
	return &trackedObjectID, nil
}
