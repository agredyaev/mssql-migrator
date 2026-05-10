package runreport

import (
	"time"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/failures"
	"reporting-db-migrations/internal/reports"
)

const (
	ValidationFailurePhase = "validation_failed"
	resultFailed           = "failed"
	resultSuccess          = "success"
)

func WriteMigration(dir string, report contracts.MigrationReport) error {
	return reports.WriteMigration(dir, report)
}

func WriteValidation(dir string, report contracts.ValidationReport) error {
	return reports.WriteValidation(dir, report)
}

func WriteValidationOutcome(dir string, report contracts.ValidationReport, err error) error {
	if writeErr := WriteValidation(dir, report); writeErr != nil {
		return contracts.Wrap(contracts.ErrCriticalState, writeErr)
	}
	return err
}

func WriteMigrationFailure(cfg config.Config, report contracts.MigrationReport, phase string, base error, cause error) error {
	report = FinalizeMigrationFailure(cfg, report, phase, base, cause)
	return writeFailureReport(func() error {
		return reports.WriteMigration(cfg.ReportDir, report)
	}, base, cause)
}

func WriteValidationFailure(cfg config.Config, report contracts.ValidationReport, phase string, base error, cause error) error {
	report = FinalizeValidationFailure(cfg, report, phase, base, cause)
	return writeFailureReport(func() error {
		return reports.WriteValidation(cfg.ReportDir, report)
	}, base, cause)
}

func FinalizeMigrationFailure(cfg config.Config, report contracts.MigrationReport, phase string, base error, cause error) contracts.MigrationReport {
	finish := failedFinish(cfg, phase, base, cause)
	report.Result = resultFailed
	report.FinishedAt = finish.finishedAt
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	report.Failed = &finish.failure
	return report
}

func FinalizeMigrationSuccess(report *contracts.MigrationReport) {
	if report == nil {
		return
	}
	report.Result = resultSuccess
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
}

func FinalizeValidationFailure(cfg config.Config, report contracts.ValidationReport, phase string, base error, cause error) contracts.ValidationReport {
	finish := failedFinish(cfg, phase, base, cause)
	report.Result = resultFailed
	report.FinishedAt = finish.finishedAt
	report.Failed = &finish.failure
	return report
}

func FinalizeValidationFailureFromReport(report contracts.ValidationReport, phase string, base error, cause error) contracts.ValidationReport {
	return FinalizeValidationFailure(config.Config{SQLRoot: report.SQLRoot, SQLBase: report.Base}, report, phase, base, cause)
}

func ReturnFailure(base error, cause error) error {
	if base == nil {
		return cause
	}
	if cause == nil || cause == base {
		return base
	}
	return contracts.Wrap(base, cause)
}

func writeFailureReport(write func() error, base error, cause error) error {
	if err := write(); err != nil {
		return contracts.Wrap(contracts.ErrCriticalState, err)
	}
	return ReturnFailure(base, cause)
}

type failedReportFinish struct {
	finishedAt time.Time
	failure    contracts.Failure
}

func failedFinish(cfg config.Config, phase string, base error, cause error) failedReportFinish {
	outcome := failures.EvaluateWithCause(cfg, phase, base, cause)
	return failedReportFinish{finishedAt: time.Now().UTC(), failure: outcome.Failure}
}
