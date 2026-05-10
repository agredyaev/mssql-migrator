package migrator

import (
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
)

func transactionModeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction {
		return config.TransactionModeNone
	}
	return defaultMode
}

func noTransactionForObject(defaultMode string, noTransaction bool) bool {
	return noTransaction || defaultMode == config.TransactionModeNone
}

func rollbackScope(defaultMode string) string {
	if defaultMode == config.TransactionModeNone {
		return contracts.RollbackScopeNone
	}
	return contracts.RollbackScopeScript
}

func rollbackScopeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction || defaultMode == config.TransactionModeNone {
		return contracts.RollbackScopeNone
	}
	return contracts.RollbackScopeScript
}
