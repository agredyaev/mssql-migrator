package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/parser"
)

func (r Runner) Baseline(ctx context.Context) error {
	baselineRunner := r
	baselineRunner.cfg.UpdatePolicy = config.UpdatePolicyNone
	session, err := baselineRunner.startProtectedSession(ctx)
	if err != nil {
		return session.Fail("baseline_failed", err, nil)
	}
	defer session.Close()
	conn := session.conn
	report := session.MigrationReport()
	if err := session.BootstrapMetadata(ctx); err != nil {
		return session.Fail("baseline_failed", contracts.ErrCriticalState, err)
	}
	successfulByKey, err := session.LoadSuccessfulChecksums(ctx)
	if err != nil {
		return session.Fail("baseline_failed", contracts.ErrCriticalState, err)
	}
	layout, hash, err := session.ResolvePlanningLayout()
	if err != nil {
		return session.Fail("baseline_failed", contracts.ErrInvalidInput, err)
	}
	session.SetLayoutHash(hash)
	plan, err := session.BuildPlan(ctx, successfulByKey, layout, hash)
	if err != nil {
		return session.Fail("baseline_failed", contracts.ErrInvalidInput, err)
	}
	runID, recorder, err := session.StartRun(ctx, contracts.CommandBaseline, "", "", contracts.RollbackScope(baselineRunner.cfg.TransactionMode))
	if err != nil {
		return session.Fail("baseline_failed", contracts.ErrCriticalState, err)
	}
	itemIDs, err := recorder.scope.Migration(ctx, plan)
	if err != nil {
		session.RecordRunFailure(ctx, recorder, contracts.ErrCriticalState, err)
		return session.Fail("baseline_failed", contracts.ErrCriticalState, err)
	}
	plannedByKey := map[string]contracts.PlannedObject{}
	for _, object := range plan.Objects {
		plannedByKey[object.NormalizedKey] = object
		itemID := lookupItemID(itemIDs, object.NormalizedKey)
		switch object.PlannedAction {
		case contracts.ActionSkipUnchanged, contracts.ActionAdoptExisting, contracts.ActionCreateObject:
			continue
		case contracts.ActionUpdateExistingModule, contracts.ActionUpdateExistingSupported, contracts.ActionReprocessChangedBlocked:
			failure := fmt.Errorf("%w: baseline found existing metadata drift for %s; use repair-checksum", contracts.ErrMetadataDrift, object.ObjectPath)
			if recordErr := recorder.attempt.ObjectFailure(ctx, baselineFailureMetadataObject(object, itemID, baselineRunner.cfg.TransactionMode), failure, true); recordErr != nil {
				r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
			}
			session.RecordRunFailure(ctx, recorder, failure, nil)
			return session.Fail("baseline_failed", failure, nil)
		default:
			failure := fmt.Errorf("%w: unsupported baseline object state %s for %s", contracts.ErrInvalidInput, object.PlannedAction, object.ObjectPath)
			if recordErr := recorder.attempt.ObjectFailure(ctx, baselineFailureMetadataObject(object, itemID, baselineRunner.cfg.TransactionMode), failure, true); recordErr != nil {
				r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
			}
			session.RecordRunFailure(ctx, recorder, failure, nil)
			return session.Fail("baseline_failed", failure, nil)
		}
	}
	if err := verifyBaselineCreatePermissions(ctx, conn, plan, layout); err != nil {
		var failure *baselinePreflightFailure
		if errors.As(err, &failure) {
			if strings.TrimSpace(failure.schemaName) != "" {
				if recordErr := recorder.attempt.SchemaFailure(ctx, failure.schemaName, err, true); recordErr != nil {
					r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
				}
			}
			if strings.TrimSpace(failure.object.Path) != "" {
				itemID := lookupItemID(itemIDs, failure.object.NormalizedKey)
				planned := plannedByKey[failure.object.NormalizedKey]
				if planned.NormalizedKey == "" {
					planned = contracts.PlannedObject{ObjectPath: failure.object.Path, NormalizedKey: failure.object.NormalizedKey, Checksum: failure.object.Checksum}
				}
				if recordErr := recorder.attempt.ObjectFailure(ctx, baselineFailureMetadataObject(planned, itemID, baselineRunner.cfg.TransactionMode), err, true); recordErr != nil {
					r.log.Warn("baseline_metadata_write_failed", logger.Redact(recordErr.Error()))
				}
			}
		}
		session.RecordRunFailure(ctx, recorder, err, nil)
		return session.Fail("baseline_failed", err, nil)
	}
	if err := baselineRunner.executePlanTracked(ctx, conn, layout, plan, report, runID, itemIDs); err != nil {
		if writeErr := session.WriteMigrationReport(); writeErr != nil {
			return contracts.Wrap(contracts.ErrCriticalState, writeErr)
		}
		session.RecordRunFailure(ctx, recorder, err, nil)
		return err
	}
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if err := session.FinishRun(ctx, recorder); err != nil {
		return session.Fail("baseline_failed", contracts.ErrCriticalState, err)
	}
	return session.WriteMigrationReport()
}

func (r Runner) RepairChecksum(ctx context.Context) error {
	session, err := r.startProtectedSession(ctx)
	if err != nil {
		return session.Fail("repair_checksum_failed", err, nil)
	}
	defer session.Close()
	report := session.MigrationReport()
	if err := session.BootstrapMetadata(ctx); err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
	}
	successfulByKey, err := session.LoadSuccessfulChecksums(ctx)
	if err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
	}
	layout, hash, err := session.ResolvePlanningLayout()
	if err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrInvalidInput, err)
	}
	session.SetLayoutHash(hash)
	target, err := resolveRepairObject(layout, r.cfg.RepairTarget)
	if err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrInvalidInput, err)
	}
	plan, err := session.BuildPlan(ctx, successfulByKey, layout, hash)
	if err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
	}
	plannedTarget, err := resolveRepairPlanObject(plan, target.NormalizedKey)
	if err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrInvalidInput, err)
	}
	if err := validateRepairEligibility(target, plannedTarget); err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrInvalidInput, err)
	}
	catalog, err := session.ReadPlanningCatalog(ctx)
	if err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
	}
	if _, exists := catalog.Objects[target.NormalizedKey]; !exists {
		return session.Fail("repair_checksum_failed", contracts.ErrInvalidInput, fmt.Errorf("repair target is missing from the database: %s", target.Path))
	}
	currentChecksum, exists := successfulByKey[target.NormalizedKey]
	if !exists {
		return session.Fail("repair_checksum_failed", contracts.ErrInvalidInput, fmt.Errorf("repair target has no successful metadata row: %s", target.Path))
	}
	_, recorder, err := session.StartRun(ctx, contracts.CommandRepairChecksum, "", "", contracts.RollbackScopeNone)
	if err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
	}
	itemID, err := recorder.scope.Repair(ctx, target, plannedTarget, catalog, currentChecksum)
	if err != nil {
		session.RecordRunFailure(ctx, recorder, contracts.ErrCriticalState, err)
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
	}
	if currentChecksum == target.Checksum {
		report.Skipped = append(report.Skipped, contracts.ScriptResult{Script: target.Path, Type: target.Kind, Checksum: target.Checksum, Reason: "checksum_already_matches"})
		report.Result = "success"
		report.FinishedAt = time.Now().UTC()
		report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
		if err := session.FinishRun(ctx, recorder); err != nil {
			return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
		}
		return session.WriteMigrationReport()
	}
	err = recorder.repair.recordSuccess(ctx, target, itemID)
	if err != nil {
		session.RecordRunFailure(ctx, recorder, contracts.ErrCriticalState, err)
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
	}
	report.Applied = append(report.Applied, contracts.ScriptResult{Script: target.Path, Type: target.Kind, Checksum: target.Checksum})
	report.Result = "success"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if err := session.FinishRun(ctx, recorder); err != nil {
		return session.Fail("repair_checksum_failed", contracts.ErrCriticalState, err)
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
