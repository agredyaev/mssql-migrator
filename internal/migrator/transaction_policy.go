package migrator

import "reporting-db-migrations/internal/contracts"

const transactionModeNone = "none"

func transactionModeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction {
		return transactionModeNone
	}
	return defaultMode
}

func noTransactionForObject(defaultMode string, noTransaction bool) bool {
	return noTransaction || defaultMode == transactionModeNone
}

func rollbackScope(defaultMode string) string {
	if defaultMode == transactionModeNone {
		return contracts.RollbackScopeNone
	}
	return contracts.RollbackScopeScript
}

func rollbackScopeForObject(defaultMode string, noTransaction bool) string {
	if noTransaction || defaultMode == transactionModeNone {
		return contracts.RollbackScopeNone
	}
	return contracts.RollbackScopeScript
}
