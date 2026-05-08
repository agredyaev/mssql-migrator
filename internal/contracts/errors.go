package contracts

import "errors"

var (
	ErrConfig           = errors.New("configuration error")
	ErrConnection       = errors.New("connection failed")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrSQLExecution     = errors.New("sql execution failed")
	ErrValidation       = errors.New("validation failed")
	ErrLockTimeout      = errors.New("lock timeout")
	ErrInvalidInput     = errors.New("invalid input")
	ErrCriticalState    = errors.New("critical metadata state")
)
