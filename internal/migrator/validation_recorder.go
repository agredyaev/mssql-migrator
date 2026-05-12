package migrator

import (
	"context"

	"reporting-db-migrations/internal/attempts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
)

type validationRecorder struct {
	writer  metadataWriter
	attempt attemptRecorder
}

func (r validationRecorder) recordFailure(ctx context.Context, objects []parser.Object, itemIDs map[string]int64, validationErr error, includeChecks bool, log logger.Logger) {
	errorMessage := attempts.RedactError(validationErr)
	for _, object := range objects {
		planned := validationMetadataObject(object, lookupItemID(itemIDs, object.NormalizedKey))
		if err := r.attempt.ObjectFailure(ctx, planned, validationErr, true); err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
	if includeChecks {
		if err := r.writer.insertAttempt(ctx, attempts.ValidationCheckFailure(errorMessage, r.writer.cfg)); err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
}

func (r validationRecorder) markSuccesses(ctx context.Context, objects []parser.Object) error {
	results := make([]metadata.ItemResult, 0, len(objects))
	for _, object := range objects {
		results = append(results, metadata.ItemResult{NormalizedKey: object.NormalizedKey, Success: true})
	}
	return r.writer.updateObjectResults(ctx, results)
}
