package migrator

import (
	"context"

	"reporting-db-migrations/internal/attempts"
	"reporting-db-migrations/internal/parser"
)

type repairRecorder struct {
	writer metadataWriter
}

func (r repairRecorder) recordSuccess(ctx context.Context, object parser.Object, itemID *int64) error {
	if err := r.writer.insertAttempt(ctx, attempts.RepairSuccess(object, itemID, r.writer.cfg)); err != nil {
		return err
	}
	return r.writer.updateObject(ctx, object.NormalizedKey, true, "")
}
