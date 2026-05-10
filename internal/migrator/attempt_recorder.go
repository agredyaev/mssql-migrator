package migrator

import (
	"context"
	"fmt"
	"strings"

	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
)

type attemptRecorder struct {
	writer metadataWriter
}

func (r attemptRecorder) SchemaSuccess(ctx context.Context, schemaName string, action string, writeAttempt bool) error {
	normalizedSchemaName := strings.ToLower(strings.TrimSpace(schemaName))
	if err := r.writer.updateSchema(ctx, normalizedSchemaName, true, ""); err != nil {
		return err
	}
	if !writeAttempt {
		return nil
	}
	return r.writer.insertAttempt(ctx, metadata.AttemptRecord{
		ScriptName:    schemaName,
		ScriptType:    contracts.ScriptTypeSchema,
		Checksum:      "-",
		Action:        action,
		ExecutionMS:   0,
		Success:       true,
		GitCommit:     r.writer.cfg.GitCommit,
		GitBranch:     r.writer.cfg.GitBranch,
		PipelineRunID: r.writer.cfg.PipelineRunID,
		PipelineURL:   logger.Redact(r.writer.cfg.PipelineURL),
		AppliedBy:     r.writer.cfg.Actor,
	})
}

func (r attemptRecorder) SchemaFailure(ctx context.Context, schemaName string, failure error, writeAttempt bool) error {
	message := logger.Redact(failure.Error())
	normalizedSchemaName := strings.ToLower(strings.TrimSpace(schemaName))
	if err := r.writer.updateSchema(ctx, normalizedSchemaName, false, message); err != nil {
		return err
	}
	if !writeAttempt {
		return nil
	}
	return r.writer.insertAttempt(ctx, metadata.AttemptRecord{
		ScriptName:    schemaName,
		ScriptType:    contracts.ScriptTypeSchema,
		Checksum:      "-",
		Action:        contracts.ActionFail,
		ExecutionMS:   0,
		Success:       false,
		ErrorMessage:  message,
		GitCommit:     r.writer.cfg.GitCommit,
		GitBranch:     r.writer.cfg.GitBranch,
		PipelineRunID: r.writer.cfg.PipelineRunID,
		PipelineURL:   logger.Redact(r.writer.cfg.PipelineURL),
		AppliedBy:     r.writer.cfg.Actor,
	})
}

func (r attemptRecorder) ObjectSuccess(ctx context.Context, object plannedMetadataObject, writeAttempt bool) error {
	if writeAttempt {
		if err := r.writer.insertAttempt(ctx, object.successAttempt(r.writer.cfg)); err != nil {
			return fmt.Errorf("critical metadata failure after %s: database object state may drift from metadata: %w", object.action(), err)
		}
	}
	return r.writer.updateObject(ctx, object.normalizedKey(), true, "")
}

func (r attemptRecorder) ObjectFailure(ctx context.Context, object plannedMetadataObject, failure error, writeAttempt bool) error {
	message := logger.Redact(failure.Error())
	if writeAttempt {
		attempt := object.failureAttempt(r.writer.cfg, message)
		if err := r.writer.insertAttempt(ctx, attempt); err != nil {
			return err
		}
	}
	return r.writer.updateObject(ctx, object.normalizedKey(), false, message)
}
