package migrator

import (
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/failures"
	"reporting-db-migrations/internal/reports"
)

type FailureReporter struct {
	cfg config.Config
}

func (r FailureReporter) Migration(report contracts.MigrationReport, phase string, base error, cause error) error {
	report = finalizeMigrationFailureReport(r.cfg, report, phase, base, cause)
	return writeFailureReport(func() error {
		return reports.WriteMigration(r.cfg.ReportDir, report)
	}, base, cause)
}

func (r FailureReporter) Validation(report contracts.ValidationReport, phase string, base error, cause error) error {
	report = finalizeValidationFailureReport(r.cfg, report, phase, base, cause)
	return writeFailureReport(func() error {
		return reports.WriteValidation(r.cfg.ReportDir, report)
	}, base, cause)
}

func writeMigrationReport(dir string, report contracts.MigrationReport) error {
	return reports.WriteMigration(dir, report)
}

func writeValidationReport(dir string, report contracts.ValidationReport) error {
	return reports.WriteValidation(dir, report)
}

func finalizeMigrationFailureReport(cfg config.Config, report contracts.MigrationReport, phase string, base error, cause error) contracts.MigrationReport {
	report.Result = "failed"
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	outcome := failures.EvaluateWithCause(cfg, phase, base, cause)
	report.Failed = &outcome.Failure
	return report
}

func finalizeValidationFailureReport(cfg config.Config, report contracts.ValidationReport, phase string, base error, cause error) contracts.ValidationReport {
	report.Result = "failed"
	report.FinishedAt = time.Now().UTC()
	outcome := failures.EvaluateWithCause(cfg, phase, base, cause)
	report.Failed = &outcome.Failure
	return report
}

func writeFailureReport(write func() error, base error, cause error) error {
	if err := write(); err != nil {
		return contracts.Wrap(contracts.ErrCriticalState, err)
	}
	return returnFailure(base, cause)
}

func returnFailure(base error, cause error) error {
	if base == nil {
		return cause
	}
	if cause == nil || cause == base {
		return base
	}
	return contracts.Wrap(base, cause)
}
