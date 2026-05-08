package planner

import (
	"fmt"
	"time"

	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/parser"
	"reporting-db-migrations/internal/reports"
	"reporting-db-migrations/internal/state"
)

func Build(cfg config.Config, st state.State) (contracts.MigrationPlan, error) {
	hash, err := checksum.SQLDirHash(cfg.SQLDir)
	if err != nil {
		return contracts.MigrationPlan{}, fmt.Errorf("%w: %v", contracts.ErrInvalidInput, err)
	}
	versioned, repeatable, _, err := parser.Discover(cfg.SQLDir)
	if err != nil {
		return contracts.MigrationPlan{}, fmt.Errorf("%w: %v", contracts.ErrInvalidInput, err)
	}
	plan := newPlan(cfg, hash)
	planVersioned(&plan, versioned, st)
	planRepeatable(&plan, repeatable, st)
	return plan, nil
}

func VerifyApprovedPlan(cfg config.Config, current string) error {
	p, err := reports.ReadPlan(cfg.PlanFile)
	if err != nil {
		return err
	}
	if p.Blocked {
		return fmt.Errorf("approved plan is blocked")
	}
	mm := []string{}
	if p.GitCommit != cfg.GitCommit {
		mm = append(mm, "git_commit")
	}
	if p.SQLDirHash != current {
		mm = append(mm, "sql_dir_hash")
	}
	if p.TargetEnv != cfg.Env {
		mm = append(mm, "target_env")
	}
	if p.TargetDatabase != cfg.Database {
		mm = append(mm, "target_database")
	}
	if p.ToolVersion != cfg.ToolVersion {
		mm = append(mm, "tool_version")
	}
	if len(mm) > 0 {
		return fmt.Errorf("plan artifact does not match current deployment input: %v", mm)
	}
	return nil
}

func newPlan(cfg config.Config, hash string) contracts.MigrationPlan {
	return contracts.MigrationPlan{
		Tool:                     "reporting-migrator",
		ToolVersion:              cfg.ToolVersion,
		GitCommit:                cfg.GitCommit,
		GitBranch:                cfg.GitBranch,
		SQLDirHash:               hash,
		TargetEnv:                cfg.Env,
		TargetDatabase:           cfg.Database,
		PlannedAt:                time.Now().UTC(),
		PendingScripts:           []contracts.ScriptResult{},
		ChangedRepeatableScripts: []contracts.ScriptResult{},
		Skipped:                  []contracts.ScriptResult{},
		BlockReasons:             []string{},
	}
}

func planVersioned(plan *contracts.MigrationPlan, scripts []parser.Script, st state.State) {
	for _, script := range scripts {
		latest, ok := st.SuccessByScript[script.Name]
		if !ok {
			plan.PendingScripts = append(plan.PendingScripts, toScriptResult(script, ""))
			continue
		}
		if latest.Checksum != script.Checksum {
			plan.Blocked = true
			plan.BlockReasons = append(plan.BlockReasons, "checksum mismatch: "+script.Name)
			continue
		}
		plan.Skipped = append(plan.Skipped, toScriptResult(script, "already_applied"))
	}
}

func planRepeatable(plan *contracts.MigrationPlan, scripts []parser.Script, st state.State) {
	for _, script := range scripts {
		latest, ok := st.SuccessByScript[script.Name]
		if !ok {
			plan.PendingScripts = append(plan.PendingScripts, toScriptResult(script, ""))
			continue
		}
		if latest.Checksum != script.Checksum {
			plan.ChangedRepeatableScripts = append(plan.ChangedRepeatableScripts, toScriptResult(script, ""))
			continue
		}
		plan.Skipped = append(plan.Skipped, toScriptResult(script, "unchanged"))
	}
}
func toScriptResult(s parser.Script, reason string) contracts.ScriptResult {
	return contracts.ScriptResult{Script: s.Name, Type: string(s.Type), Checksum: s.Checksum, Reason: reason}
}
