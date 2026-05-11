package migrator

import (
	"context"
	"database/sql"
	"fmt"

	"reporting-db-migrations/internal/attempts"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
)

type metadataRecorder struct {
	writer     metadataWriter
	attempt    attemptRecorder
	scope      scopeWriter
	repair     repairRecorder
	validation validationRecorder
	conn       *sql.Conn
}

func newMetadataRecorder(cfg config.Config, execer metadata.Execer, conn *sql.Conn, runID string) metadataRecorder {
	writer := newMetadataWriter(cfg, execer, runID)
	attempt := attemptRecorder{writer: writer}
	return metadataRecorder{
		writer:     writer,
		attempt:    attempt,
		scope:      scopeWriter{writer: writer, conn: conn},
		repair:     repairRecorder{writer: writer},
		validation: validationRecorder{writer: writer, attempt: attempt},
		conn:       conn,
	}
}

func (r metadataRecorder) recordRunResult(ctx context.Context, success bool, base error, cause error) error {
	return r.writer.finishRun(ctx, success, base, cause)
}

func (r metadataRecorder) loadObjectItemIDs(ctx context.Context) (map[string]int64, error) {
	if r.conn == nil {
		return nil, fmt.Errorf("load object item ids: missing connection")
	}
	return r.writer.loadObjectItemIDs(ctx, r.conn)
}

type plannedMetadataObject struct {
	objectPath       string
	kind             string
	normalizedKeyVal string
	checksum         string
	actionValue      string
	itemID           *int64
	executionMS      int
	transactionMode  string
	rollbackScope    string
	noTransaction    bool
}

func passiveMetadataObject(planned contracts.PlannedObject, itemID *int64) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       planned.ObjectPath,
		kind:             planned.Kind,
		normalizedKeyVal: planned.NormalizedKey,
		checksum:         planned.Checksum,
		actionValue:      planned.PlannedAction,
		itemID:           itemID,
		transactionMode:  planned.TransactionMode,
		rollbackScope:    planned.RollbackScope,
		noTransaction:    planned.NoTransaction,
	}
}

func executedMetadataObject(object parser.Object, planned contracts.PlannedObject, itemID *int64, executionMS int) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       object.Path,
		kind:             object.Kind,
		normalizedKeyVal: object.NormalizedKey,
		checksum:         object.Checksum,
		actionValue:      planned.PlannedAction,
		itemID:           itemID,
		executionMS:      executionMS,
		transactionMode:  planned.TransactionMode,
		rollbackScope:    planned.RollbackScope,
		noTransaction:    planned.NoTransaction,
	}
}

func validationMetadataObject(object parser.Object, itemID *int64) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       object.Path,
		kind:             object.Kind,
		normalizedKeyVal: object.NormalizedKey,
		checksum:         object.Checksum,
		actionValue:      validationObjectAction(object.Kind),
		itemID:           itemID,
		executionMS:      0,
		transactionMode:  config.TransactionModeNone,
		rollbackScope:    contracts.RollbackScopeNone,
		noTransaction:    true,
	}
}

func validationObjectAction(kind string) string {
	if parser.IsModuleKind(kind) {
		return contracts.ActionValidateChecked
	}
	return contracts.ActionValidateSkipped
}

func baselineFailureMetadataObject(planned contracts.PlannedObject, itemID *int64, defaultMode string) plannedMetadataObject {
	return plannedMetadataObject{
		objectPath:       planned.ObjectPath,
		kind:             planned.Kind,
		normalizedKeyVal: planned.NormalizedKey,
		checksum:         planned.Checksum,
		actionValue:      planned.PlannedAction,
		itemID:           itemID,
		executionMS:      0,
		transactionMode:  transactionModeForObject(defaultMode, false),
		rollbackScope:    rollbackScope(defaultMode),
		noTransaction:    noTransactionForObject(defaultMode, false),
	}
}

func (o plannedMetadataObject) action() string {
	return o.actionValue
}

func (o plannedMetadataObject) normalizedKey() string {
	return o.normalizedKeyVal
}

func (o plannedMetadataObject) successAttempt(cfg config.Config) metadata.AttemptRecord {
	attempt := metadata.AttemptRecord{
		ItemID:          o.itemID,
		ScriptName:      o.normalizedKeyVal,
		Checksum:        o.checksum,
		Action:          o.actionValue,
		ExecutionMS:     o.executionMS,
		Success:         true,
		TransactionMode: o.transactionMode,
		RollbackScope:   o.rollbackScope,
		NoTransaction:   o.noTransaction,
	}
	attempts.ApplyAuditFields(cfg, &attempt)
	return attempt
}

func (o plannedMetadataObject) failureAttempt(cfg config.Config, message string) metadata.AttemptRecord {
	attempt := o.successAttempt(cfg)
	attempt.Action = contracts.ActionFail
	attempt.Success = false
	attempt.ErrorMessage = message
	return attempt
}
