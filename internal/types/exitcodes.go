package types

const (
	ExitOK = 0

	ExitGeneralError = 1
	ExitConfigError  = 2
	ExitConnError    = 3

	ExitChecksumMismatch = 4
	ExitSQLExecution     = 5
	ExitValidation       = 6
	ExitLockTimeout      = 7
	ExitInvalidInput     = 8
	ExitCriticalState    = 9
)
