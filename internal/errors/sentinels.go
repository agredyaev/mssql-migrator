package errors

import "errors"

var (
	ErrConfig                  = errors.New("configuration error")
	ErrConnection              = errors.New("connection failed")
	ErrChecksumMismatch        = errors.New("checksum mismatch")
	ErrSQLExecution            = errors.New("sql execution failed")
	ErrValidation              = errors.New("validation failed")
	ErrLockTimeout             = errors.New("lock timeout")
	ErrLockFailed              = errors.New("lock failed")
	ErrInvalidInput            = errors.New("invalid input")
	ErrCriticalState           = errors.New("critical metadata state")
	ErrApprovedPlanMissing     = errors.New("approved plan missing")
	ErrApprovedPlanMismatch    = errors.New("approved plan mismatch")
	ErrMetadataDrift           = errors.New("metadata drift")
	ErrMissingSchemaPermission = errors.New("missing schema creation permission")
	ErrMissingObjectPermission = errors.New("missing object DDL permission")
	ErrMissingParentObject     = errors.New("missing parent object")
	ErrPlanBlocked             = errors.New("plan is blocked")
	ErrMissingDDLPermission    = errors.New("missing metadata DDL permission")
	ErrSchemaIncompatible      = errors.New("metadata schema incompatible")
)
