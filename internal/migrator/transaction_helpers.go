package migrator

import "reporting-db-migrations/internal/config"

func rollbackScope(defaultMode string) string {
	if defaultMode == config.TransactionModeNone {
		return "none"
	}
	return "script"
}

func transactionModeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction {
		return config.TransactionModeNone
	}
	return defaultMode
}

func rollbackScopeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction || defaultMode == config.TransactionModeNone {
		return "none"
	}
	return "script"
}
