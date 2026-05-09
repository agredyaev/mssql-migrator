package migrator

import (
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/reports"
)

func writeMigrationReport(dir string, report contracts.MigrationReport) error {
	return reports.WriteMigration(dir, report)
}

func writeValidationReport(dir string, report contracts.ValidationReport) error {
	return reports.WriteValidation(dir, report)
}
