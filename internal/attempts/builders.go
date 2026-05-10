package attempts

import (
	"strings"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/metadata"
	"reporting-db-migrations/internal/parser"
)

func ApplyAuditFields(cfg config.Config, attempt *metadata.AttemptRecord) {
	if attempt == nil {
		return
	}
	attempt.GitCommit = cfg.GitCommit
	attempt.GitBranch = cfg.GitBranch
	attempt.PipelineRunID = cfg.PipelineRunID
	attempt.PipelineURL = logger.Redact(cfg.PipelineURL)
	attempt.AppliedBy = cfg.Actor
}

func Schema(schemaName string, action string, success bool, message string, cfg config.Config) metadata.AttemptRecord {
	attempt := metadata.AttemptRecord{
		ScriptName:   schemaName,
		ScriptType:   contracts.ScriptTypeSchema,
		Checksum:     "-",
		Action:       action,
		ExecutionMS:  0,
		Success:      success,
		ErrorMessage: strings.TrimSpace(message),
	}
	ApplyAuditFields(cfg, &attempt)
	return attempt
}

func ValidationCheckFailure(message string, cfg config.Config) metadata.AttemptRecord {
	attempt := metadata.AttemptRecord{
		ScriptName:       "validation/checks",
		ScriptType:       contracts.ScriptTypeValidate,
		Checksum:         "-",
		Action:           contracts.ActionFail,
		ExecutionMS:      0,
		Success:          false,
		ErrorMessage:     strings.TrimSpace(message),
		TransactionMode:  config.TransactionModeNone,
		TransactionScope: config.TransactionModeNone,
		RollbackScope:    contracts.RollbackScopeNone,
		NoTransaction:    true,
	}
	ApplyAuditFields(cfg, &attempt)
	return attempt
}

func RepairSuccess(object parser.Object, itemID *int64, cfg config.Config) metadata.AttemptRecord {
	attempt := metadata.AttemptRecord{
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
	}
	ApplyAuditFields(cfg, &attempt)
	return attempt
}

func RedactError(err error) string {
	if err == nil {
		return ""
	}
	return logger.Redact(err.Error())
}
