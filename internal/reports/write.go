package reports

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

func WritePlan(dir string, plan contracts.MigrationPlan) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	plan = redactPlan(plan)
	if err := writeJSONAtomic(filepath.Join(dir, "migration-plan.json"), plan); err != nil {
		return err
	}
	text := formatPlanText(plan)
	return writeTextAtomic(filepath.Join(dir, "migration-plan.txt"), text)
}

func WriteMigration(dir string, report contracts.MigrationReport) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	report = redactMigrationReport(report)
	if err := writeJSONAtomic(filepath.Join(dir, "migration-report.json"), report); err != nil {
		return err
	}
	failure := ""
	if report.Failed != nil {
		failure = report.Failed.Error
	}
	text := formatMigrationText(report, failure)
	return writeTextAtomic(filepath.Join(dir, "migration-report.txt"), text)
}

func WriteValidation(dir string, report contracts.ValidationReport) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	report = redactValidationReport(report)
	if err := writeJSONAtomic(filepath.Join(dir, "validation-report.json"), report); err != nil {
		return err
	}
	failure := ""
	if report.Failed != nil {
		failure = report.Failed.Error
	}
	text := formatValidationText(report, failure)
	return writeTextAtomic(filepath.Join(dir, "validation-report.txt"), text)
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

func writeJSONAtomic(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeFileAtomic(path, b)
}

func writeTextAtomic(path string, value string) error {
	return writeFileAtomic(path, []byte(value))
}

func writeFileAtomic(path string, content []byte) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func redactPlan(plan contracts.MigrationPlan) contracts.MigrationPlan {
	plan.Failures = redactStrings(plan.Failures)
	plan.Blockers = redactStrings(plan.Blockers)
	plan.BlockReasons = redactStrings(plan.BlockReasons)
	return plan
}

func redactMigrationReport(report contracts.MigrationReport) contracts.MigrationReport {
	report.PipelineURL = logger.Redact(report.PipelineURL)
	report.Applied = redactScriptResults(report.Applied)
	report.Skipped = redactScriptResults(report.Skipped)
	if report.Failed != nil {
		failed := *report.Failed
		redactFailure(&failed)
		report.Failed = &failed
	}
	return report
}

func redactValidationReport(report contracts.ValidationReport) contracts.ValidationReport {
	report.PipelineURL = logger.Redact(report.PipelineURL)
	if report.Failed != nil {
		failed := *report.Failed
		redactFailure(&failed)
		report.Failed = &failed
	}
	return report
}

func redactScriptResults(items []contracts.ScriptResult) []contracts.ScriptResult {
	if len(items) == 0 {
		return items
	}
	result := make([]contracts.ScriptResult, len(items))
	copy(result, items)
	for i := range result {
		result[i].Reason = logger.Redact(result[i].Reason)
	}
	return result
}

func redactFailure(failure *contracts.Failure) {
	if failure == nil {
		return
	}
	failure.Script = logger.Redact(failure.Script)
	failure.Phase = logger.Redact(failure.Phase)
	failure.SQLRoot = logger.Redact(failure.SQLRoot)
	failure.Base = logger.Redact(failure.Base)
	failure.Class = logger.Redact(failure.Class)
	failure.Reason = logger.Redact(failure.Reason)
	failure.SQL = logger.Redact(failure.SQL)
	failure.Error = logger.Redact(failure.Error)
}

func redactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = logger.Redact(value)
	}
	return result
}

func formatPlanText(plan contracts.MigrationPlan) string {
	failures := "-"
	if len(plan.Failures) > 0 {
		failures = strings.Join(plan.Failures, "\n")
	}
	reasons := "-"
	if len(plan.BlockReasons) > 0 {
		reasons = strings.Join(plan.BlockReasons, "\n")
	}
	return fmt.Sprintf(
		"Plan for %s/%s\nSchema version: %s\nCommand: %s\nTool version: %s\nTool commit: %s\nGit commit: %s\nBase: %s/%s\nEffective base path: %s\nLayout hash: %s\nComparison mode: %s\nUpdate policy: %s\nTransaction mode: %s\nRollback: %s\nBlocked: %t\nSchemas: %d\nObjects: %d\nChecks: %d\nCreate: %d\nAdopt: %d\nSkip: %d\nChanged: %d\nFailures:\n%s\nBlock reasons:\n%s\n",
		plan.Target.Environment,
		plan.Target.Database,
		plan.SchemaVersion,
		plan.Command,
		plan.ToolVersion,
		plan.ToolCommit,
		plan.GitCommit,
		plan.SQLRoot,
		plan.Base,
		plan.EffectiveBasePath,
		plan.LayoutHash,
		plan.ComparisonMode,
		plan.UpdatePolicy,
		plan.TransactionMode,
		plan.Rollback,
		plan.Blocked,
		plan.Summary.SchemaCount,
		plan.Summary.ObjectCount,
		plan.Summary.CheckCount,
		plan.Summary.CreateCount,
		plan.Summary.AdoptCount,
		plan.Summary.SkipCount,
		plan.Summary.ChangedCount,
		failures,
		reasons,
	)
}

func formatMigrationText(report contracts.MigrationReport, failure string) string {
	return fmt.Sprintf(
		"Migration result: %s\nEnvironment: %s\nDatabase: %s\nValidation scope: %s\nValidation skipped: %t\nApplied: %d\nSkipped: %d\nFailure: %s\n",
		report.Result,
		report.Environment,
		report.Database,
		report.ValidationScope,
		report.ValidationSkipped,
		len(report.Applied),
		len(report.Skipped),
		failure,
	)
}

func formatValidationText(report contracts.ValidationReport, failure string) string {
	return fmt.Sprintf(
		"Validation result: %s\nCommand: %s\nScope: %s\nIncludes checks: %t\nLayout hash: %s\nEnvironment: %s\nDatabase: %s\nModules refreshed: %d\nChecks passed: %d\nChecks failed: %d\nFailure: %s\n",
		report.Result,
		report.Command,
		report.Scope,
		report.IncludesChecks,
		report.LayoutHash,
		report.Environment,
		report.Database,
		report.Validation.ModulesRefreshed,
		report.Validation.ChecksPassed,
		report.Validation.ChecksFailed,
		failure,
	)
}
