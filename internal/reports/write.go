package reports

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reporting-db-migrations/internal/contracts"
)

func WritePlan(dir string, plan contracts.MigrationPlan) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "migration-plan.json"), plan); err != nil {
		return err
	}
	text := formatPlanText(plan)
	return os.WriteFile(filepath.Join(dir, "migration-plan.txt"), []byte(text), 0o644)
}

func WriteMigration(dir string, report contracts.MigrationReport) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "migration-report.json"), report); err != nil {
		return err
	}
	failure := ""
	if report.Failed != nil {
		failure = report.Failed.Error
	}
	text := formatMigrationText(report, failure)
	return os.WriteFile(filepath.Join(dir, "migration-report.txt"), []byte(text), 0o644)
}

func WriteValidation(dir string, report contracts.ValidationReport) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "validation-report.json"), report); err != nil {
		return err
	}
	failure := ""
	if report.Failed != nil {
		failure = report.Failed.Error
	}
	text := formatValidationText(report, failure)
	return os.WriteFile(filepath.Join(dir, "validation-report.txt"), []byte(text), 0o644)
}

func ReadPlan(path string) (contracts.MigrationPlan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return contracts.MigrationPlan{}, err
	}
	var plan contracts.MigrationPlan
	if err := json.Unmarshal(b, &plan); err != nil {
		return contracts.MigrationPlan{}, err
	}
	return plan, nil
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func formatPlanText(plan contracts.MigrationPlan) string {
	return fmt.Sprintf(
		"Plan for %s/%s\nBlocked: %t\nPending: %d\nChanged repeatable: %d\n%s\n",
		plan.TargetEnv,
		plan.TargetDatabase,
		plan.Blocked,
		len(plan.PendingScripts),
		len(plan.ChangedRepeatableScripts),
		strings.Join(plan.BlockReasons, "\n"),
	)
}

func formatMigrationText(report contracts.MigrationReport, failure string) string {
	return fmt.Sprintf(
		"Migration result: %s\nEnvironment: %s\nDatabase: %s\nApplied: %d\nSkipped: %d\nFailure: %s\n",
		report.Result,
		report.Environment,
		report.Database,
		len(report.Applied),
		len(report.Skipped),
		failure,
	)
}

func formatValidationText(report contracts.ValidationReport, failure string) string {
	return fmt.Sprintf(
		"Validation result: %s\nEnvironment: %s\nDatabase: %s\nModules refreshed: %d\nChecks passed: %d\nChecks failed: %d\nFailure: %s\n",
		report.Result,
		report.Environment,
		report.Database,
		report.Validation.ModulesRefreshed,
		report.Validation.ChecksPassed,
		report.Validation.ChecksFailed,
		failure,
	)
}
