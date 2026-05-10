package migrator

import (
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
)

func setFailedResult(report *contracts.MigrationReport, script string, message string) {
	if report == nil {
		return
	}
	report.Result = reportResultFailed
	report.Failed = &contracts.Failure{Script: script, Error: logger.Redact(message)}
}

func setCriticalMetadataFailure(report *contracts.MigrationReport, script string, prefix string, err error) {
	if err == nil {
		setFailedResult(report, script, prefix)
		return
	}
	setFailedResult(report, script, prefix+logger.Redact(err.Error()))
}
