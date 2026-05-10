package migrator

import (
	"context"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
)

type validationRecorder struct {
	writer  metadataWriter
	attempt attemptRecorder
}

func (r validationRecorder) recordFailure(ctx context.Context, objects []parser.Object, itemIDs map[string]int64, validationErr error, includeChecks bool, log logger.Logger) {
	errorMessage := logger.Redact(validationErr.Error())
	for _, object := range objects {
		planned := validationMetadataObject(object, lookupItemID(itemIDs, object.NormalizedKey))
		if err := r.attempt.ObjectFailure(ctx, planned, validationErr, true); err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
	if includeChecks {
		if err := r.writer.insertAttempt(ctx, metadata.AttemptRecord{
			ScriptName:       "validation/checks",
			ScriptType:       contracts.ScriptTypeValidate,
			Checksum:         "-",
			Action:           contracts.ActionFail,
			ExecutionMS:      0,
			Success:          false,
			ErrorMessage:     errorMessage,
			TransactionMode:  config.TransactionModeNone,
			TransactionScope: config.TransactionModeNone,
			RollbackScope:    contracts.RollbackScopeNone,
			NoTransaction:    true,
			GitCommit:        r.writer.cfg.GitCommit,
			GitBranch:        r.writer.cfg.GitBranch,
			PipelineRunID:    r.writer.cfg.PipelineRunID,
			PipelineURL:      logger.Redact(r.writer.cfg.PipelineURL),
			AppliedBy:        r.writer.cfg.Actor,
		}); err != nil {
			log.Warn("validation_metadata_write_failed", logger.Redact(err.Error()))
		}
	}
}

func (r validationRecorder) recordSuccesses(ctx context.Context, objects []parser.Object) error {
	for _, object := range objects {
		if err := r.writer.updateObject(ctx, object.NormalizedKey, true, ""); err != nil {
			return err
		}
	}
	return nil
}
