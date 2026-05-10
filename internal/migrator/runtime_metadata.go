package migrator

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/failures"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
)

func (r Runner) startRun(ctx context.Context, execer metadata.Execer, command string, planFile string, planHash string, rollbackScope string) (string, error) {
	runID := uuid.NewString()
	record := metadata.RunRecord{
		RunID:             runID,
		Command:           command,
		ToolVersion:       r.cfg.ToolVersion,
		ToolCommit:        r.cfg.ToolCommit,
		SQLRoot:           r.cfg.SQLRoot,
		BaseName:          r.cfg.SQLBase,
		EffectiveBasePath: r.cfg.SelectedBasePath(),
		TargetEnvironment: r.cfg.Env,
		TargetDatabase:    r.cfg.Database,
		GitCommit:         r.cfg.GitCommit,
		GitBranch:         r.cfg.GitBranch,
		PipelineRunID:     r.cfg.PipelineRunID,
		PipelineURL:       logger.Redact(r.cfg.PipelineURL),
		AppliedBy:         r.cfg.Actor,
		ComparisonMode:    config.ComparisonModeCaseInsensitive,
		UpdatePolicy:      r.cfg.UpdatePolicy,
		TransactionMode:   r.cfg.TransactionMode,
		RollbackScope:     rollbackScope,
		PlanFile:          planFile,
		PlanHash:          planHash,
	}
	if command == contracts.CommandValidate || command == contracts.CommandRepairChecksum {
		record.ComparisonMode = ""
	}
	if command == contracts.CommandValidate {
		record.UpdatePolicy = ""
	}
	if command == contracts.CommandRepairChecksum {
		record.TransactionMode = config.TransactionModeNone
		record.RollbackScope = contracts.RollbackScopeNone
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	if err := metadata.StartRun(metaCtx, execer, record); err != nil {
		return "", err
	}
	return runID, nil
}

func finishRun(ctx context.Context, execer metadata.Execer, runID string, success bool, base error, cause error) error {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	errorClass := ""
	errorMessage := ""
	if !success {
		errorClass = failures.Classify(base, cause)
		errorMessage = failures.Message(base, cause)
	}
	metaCtx, cancel := metadataContext(ctx)
	defer cancel()
	return metadata.FinishRun(metaCtx, execer, runID, success, errorClass, errorMessage)
}

func planArtifactHash(planFile string) string {
	if strings.TrimSpace(planFile) == "" {
		return ""
	}
	hash, err := checksum.SHA256File(planFile)
	if err != nil {
		return ""
	}
	return hash
}

func boolPtr(value bool) *bool {
	return &value
}
