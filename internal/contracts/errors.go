package contracts

import "errors"

var (
	ErrConfig                  = errors.New("configuration error")
	ErrConnection              = errors.New("connection failed")
	ErrChecksumMismatch        = errors.New("checksum mismatch")
	ErrSQLExecution            = errors.New("sql execution failed")
	ErrValidation              = errors.New("validation failed")
	ErrLockTimeout             = errors.New("lock timeout")
	ErrInvalidInput            = errors.New("invalid input")
	ErrCriticalState           = errors.New("critical metadata state")
	ErrApprovedPlanMissing     = errors.New("approved plan missing")
	ErrApprovedPlanMismatch    = errors.New("approved plan mismatch")
	ErrMetadataDrift           = errors.New("metadata drift")
	ErrMissingSchemaPermission = errors.New("missing schema creation permission")
	ErrMissingObjectPermission = errors.New("missing object DDL permission")
	ErrMissingParentObject     = errors.New("missing parent object")
)

type errorWithCause struct {
	base  error
	cause error
}

func Wrap(base error, cause error) error {
	switch {
	case base == nil:
		return cause
	case cause == nil:
		return base
	default:
		return errorWithCause{base: base, cause: cause}
	}
}

func (e errorWithCause) Error() string {
	if e.cause == nil {
		return e.base.Error()
	}
	return e.base.Error() + ": " + e.cause.Error()
}

func (e errorWithCause) Unwrap() []error {
	if e.cause == nil {
		return []error{e.base}
	}
	return []error{e.base, e.cause}
}

func (e errorWithCause) FailureBase() error {
	return e.base
}

func (e errorWithCause) FailureCause() error {
	return e.cause
}
