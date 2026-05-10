package migrator

import (
	"context"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
)

type repairRecorder struct {
	writer metadataWriter
}

func (r repairRecorder) recordSuccess(ctx context.Context, object parser.Object, itemID *int64) error {
	if err := r.writer.insertAttempt(ctx, metadata.AttemptRecord{
		ItemID:           itemID,
		ScriptName:       object.NormalizedKey,
		ScriptType:       contracts.ScriptTypeRepair,
		Checksum:         object.Checksum,
		Action:           contracts.ActionRepairChecksum,
		ExecutionMS:      0,
		Success:          true,
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
		return err
	}
	return r.writer.updateObject(ctx, object.NormalizedKey, true, "")
}
