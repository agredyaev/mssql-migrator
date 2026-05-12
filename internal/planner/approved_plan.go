package planner

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/reports"
)

func ReadApprovedPlan(path string) (contracts.MigrationPlan, error) {
	plan, err := reports.ReadPlan(path)
	if err != nil {
		return contracts.MigrationPlan{}, contracts.Wrap(contracts.ErrApprovedPlanMissing, err)
	}
	if err := validateApprovedPlanShape(plan); err != nil {
		return contracts.MigrationPlan{}, err
	}
	return plan, nil
}

func VerifyApprovedPlan(cfg config.Config, current contracts.MigrationPlan) error {
	approved, err := ReadApprovedPlan(cfg.PlanFile)
	if err != nil {
		return err
	}
	return VerifyApprovedPlanMatches(cfg, approved, current)
}

func VerifyApprovedPlanMatches(cfg config.Config, approved contracts.MigrationPlan, current contracts.MigrationPlan) error {
	mm := approvalBoundaryMismatches(cfg, approved, current)
	if len(mm) > 0 {
		return contracts.Wrap(contracts.ErrApprovedPlanMismatch, fmt.Errorf("%v", mm))
	}
	if !reflect.DeepEqual(stableSchemas(approved.Schemas), stableSchemas(current.Schemas)) {
		return fmt.Errorf("%w: schema set does not match current deployment state", contracts.ErrApprovedPlanMismatch)
	}
	if !reflect.DeepEqual(stableObjects(approved.Objects), stableObjects(current.Objects)) {
		return fmt.Errorf("%w: object set does not match current deployment state", contracts.ErrApprovedPlanMismatch)
	}
	return nil
}

func validateApprovedPlanShape(plan contracts.MigrationPlan) error {
	if plan.Blocked {
		return fmt.Errorf("%w: approved plan is blocked", contracts.ErrApprovedPlanMismatch)
	}
	if plan.SchemaVersion != "v8" {
		return fmt.Errorf("%w: schema version %s", contracts.ErrApprovedPlanMismatch, plan.SchemaVersion)
	}
	if plan.Command != "plan" {
		return fmt.Errorf("%w: command %s", contracts.ErrApprovedPlanMismatch, plan.Command)
	}
	return nil
}

func approvalBoundaryMismatches(cfg config.Config, approved contracts.MigrationPlan, current contracts.MigrationPlan) []string {
	mm := []string{}
	if approved.GitCommit != cfg.GitCommit {
		mm = append(mm, "git_commit")
	}
	if approved.LayoutHash != current.LayoutHash {
		mm = append(mm, "layout_hash")
	}
	if approved.Target.Environment != current.Target.Environment {
		mm = append(mm, "target.environment")
	}
	if approved.Target.Database != current.Target.Database {
		mm = append(mm, "target.database")
	}
	if approved.ToolVersion != current.ToolVersion {
		mm = append(mm, "tool_version")
	}
	if approved.ToolCommit != current.ToolCommit {
		mm = append(mm, "tool_commit")
	}
	if approved.ComparisonMode != current.ComparisonMode {
		mm = append(mm, "comparison_mode")
	}
	if approved.UpdatePolicy != current.UpdatePolicy {
		mm = append(mm, "update_policy")
	}
	if approved.TransactionMode != current.TransactionMode {
		mm = append(mm, "transaction_mode")
	}
	if approved.Rollback != current.Rollback {
		mm = append(mm, "rollback")
	}
	if approved.SQLRoot != current.SQLRoot {
		mm = append(mm, "sql_root")
	}
	if approved.Base != current.Base {
		mm = append(mm, "base")
	}
	if approved.EffectiveBasePath != current.EffectiveBasePath {
		mm = append(mm, "effective_base_path")
	}
	return mm
}

func stableSchemas(items []contracts.PlannedSchema) []contracts.PlannedSchema {
	result := make([]contracts.PlannedSchema, len(items))
	copy(result, items)
	sort.Slice(result, func(i, j int) bool {
		left := strings.ToLower(result[i].SchemaName)
		right := strings.ToLower(result[j].SchemaName)
		if left != right {
			return left < right
		}
		if result[i].Action != result[j].Action {
			return result[i].Action < result[j].Action
		}
		return !result[i].Exists && result[j].Exists
	})
	return result
}

func stableObjects(items []contracts.PlannedObject) []contracts.PlannedObject {
	result := make([]contracts.PlannedObject, 0, len(items))
	for _, item := range items {
		result = append(result, contracts.PlannedObject{
			ObjectPath:      item.ObjectPath,
			SchemaName:      item.SchemaName,
			Kind:            item.Kind,
			ObjectName:      item.ObjectName,
			ParentName:      item.ParentName,
			NormalizedKey:   item.NormalizedKey,
			Checksum:        item.Checksum,
			PlannedAction:   item.PlannedAction,
			TransactionMode: item.TransactionMode,
			RollbackScope:   item.RollbackScope,
			NoTransaction:   item.NoTransaction,
			SourceFile:      item.SourceFile,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].NormalizedKey != result[j].NormalizedKey {
			return result[i].NormalizedKey < result[j].NormalizedKey
		}
		return result[i].ObjectPath < result[j].ObjectPath
	})
	return result
}
