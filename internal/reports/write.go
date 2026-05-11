package reports

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

func WritePlan(dir string, plan contracts.MigrationPlan) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	plan = redactPlan(plan)
	jsonContent, err := marshalJSON(plan)
	if err != nil {
		return err
	}
	text := formatPlanText(plan)
	return writeFilePairAtomic(
		filepath.Join(dir, "migration-plan.json"), jsonContent,
		filepath.Join(dir, "migration-plan.txt"), []byte(text),
	)
}

func WriteMigration(dir string, report contracts.MigrationReport) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	report = redactMigrationReport(report)
	jsonContent, err := marshalJSON(report)
	if err != nil {
		return err
	}
	failure := ""
	if report.Failed != nil {
		failure = report.Failed.Error
	}
	text := formatMigrationText(report, failure)
	return writeFilePairAtomic(
		filepath.Join(dir, "migration-report.json"), jsonContent,
		filepath.Join(dir, "migration-report.txt"), []byte(text),
	)
}

func WriteValidation(dir string, report contracts.ValidationReport) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	report = redactValidationReport(report)
	jsonContent, err := marshalJSON(report)
	if err != nil {
		return err
	}
	failure := ""
	if report.Failed != nil {
		failure = report.Failed.Error
	}
	text := formatValidationText(report, failure)
	return writeFilePairAtomic(
		filepath.Join(dir, "validation-report.json"), jsonContent,
		filepath.Join(dir, "validation-report.txt"), []byte(text),
	)
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

func MarshalPlanJSON(plan contracts.MigrationPlan) ([]byte, error) {
	return marshalJSON(redactPlan(plan))
}

func marshalJSON(value any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func writeFilePairAtomic(firstPath string, firstContent []byte, secondPath string, secondContent []byte) error {
	// Publish the human-readable companion first and the JSON artifact last.
	// The JSON file is the commit marker for readers that need a consistent pair.
	secondTemp, err := writeTempFile(secondPath, secondContent)
	if err != nil {
		return err
	}
	firstTemp, err := writeTempFile(firstPath, firstContent)
	if err != nil {
		_ = os.Remove(secondTemp)
		return err
	}
	if err := os.Rename(secondTemp, secondPath); err != nil {
		_ = os.Remove(secondTemp)
		_ = os.Remove(firstTemp)
		return err
	}
	if err := syncPathDir(secondPath); err != nil {
		_ = os.Remove(firstTemp)
		return err
	}
	if err := os.Rename(firstTemp, firstPath); err != nil {
		_ = os.Remove(firstTemp)
		return fmt.Errorf("write primary report commit marker: %w", err)
	}
	return syncPathDir(firstPath)
}

func writeTempFile(path string, content []byte) (string, error) {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", err
	}
	tmp := tmpFile.Name()
	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

func syncPathDir(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && isWindowsAccessDenied(err) {
		return nil
	}
	return err
}

func isWindowsAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	if runtime.GOOS != "windows" {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "access is denied")
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
	plan = redactPlan(plan)
	failures := "-"
	if len(plan.Failures) > 0 {
		failures = strings.Join(plan.Failures, "\n")
	}
	reasons := "-"
	if len(plan.BlockReasons) > 0 {
		reasons = strings.Join(plan.BlockReasons, "\n")
	}
	objectActions := "-"
	if len(plan.Objects) > 0 {
		lines := make([]string, 0, len(plan.Objects))
		for _, object := range plan.Objects {
			lines = append(lines, summarizePlannedObject(object))
		}
		objectActions = strings.Join(lines, "\n")
	}
	return fmt.Sprintf(
		"Plan for %s/%s\nSchema version: %s\nCommand: %s\nTool version: %s\nTool commit: %s\nGit commit: %s\nBase: %s/%s\nEffective base path: %s\nLayout hash: %s\nComparison mode: %s\nUpdate policy: %s\nTransaction mode: %s\nRollback: %s\nBlocked: %t\nSchemas: %d\nObjects: %d\nChecks: %d\nCreate: %d\nAdopt: %d\nSkip: %d\nChanged: %d\nPlan object decisions:\n%s\nFailures:\n%s\nBlock reasons:\n%s\n",
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
		objectActions,
		failures,
		reasons,
	)
}

func FormatPlanText(plan contracts.MigrationPlan) string {
	return formatPlanText(plan)
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

func summarizePlannedObject(object contracts.PlannedObject) string {
	return fmt.Sprintf("- %s [%s]: %s", object.ObjectPath, object.Kind, describePlannedAction(object))
}

func describePlannedAction(object contracts.PlannedObject) string {
	switch object.PlannedAction {
	case contracts.ActionCreateObject:
		return "create in database because the object is missing from the live catalog"
	case contracts.ActionAdoptExisting:
		return "adopt existing database object without DDL because no successful metadata checksum exists yet"
	case contracts.ActionSkipUnchanged:
		return "skip DDL because the latest successful metadata checksum already matches the current repo SQL"
	case contracts.ActionUpdateExistingModule:
		return "apply repo SQL to the existing tracked object because checksum drift was detected and module updates are allowed"
	case contracts.ActionUpdateExistingSupported:
		return "apply repo SQL to the existing tracked object because checksum drift was detected and supported existing-object updates are allowed"
	case contracts.ActionReprocessChanged:
		if object.Kind == "tables" && len(object.TransitionPaths) > 0 {
			return "apply checked-in transitions before the repo table SQL because tracked table drift was detected: " + strings.Join(object.TransitionPaths, ", ")
		}
		return "reprocess the tracked object because checksum drift was detected and the repo provides an explicit execution path"
	case contracts.ActionReprocessChangedBlocked:
		if object.Kind == "tables" {
			if len(object.TransitionPaths) > 0 {
				return "blocked because the tracked table checksum changed but the current checked-in transition set is not on an executable path"
			}
			return "blocked because the tracked table checksum changed and a checked-in transition is required under " + requiredTransitionDir(object) + " before the repo table SQL can run"
		}
		return "blocked because the object is already tracked, the repo checksum changed, and this change is not on a safe automatic DDL path"
	default:
		return object.PlannedAction
	}
}

func requiredTransitionDir(object contracts.PlannedObject) string {
	if strings.TrimSpace(object.SchemaName) == "" || strings.TrimSpace(object.ObjectName) == "" {
		return "<schema>/tables/_migrations/<table>/"
	}
	return object.SchemaName + "/tables/_migrations/" + object.ObjectName + "/"
}
