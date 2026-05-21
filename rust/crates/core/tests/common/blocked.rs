//! Blocked migrate + scaffold e2e (blocked_table_plan scenario).

use std::path::{Path, PathBuf};
use std::process::Command as OsCommand;

use migrator_core::audit::invalidate_audit_cache;
use migrator_core::config::Config;
use migrator_core::engine::{run_command, Command};
use migrator_core::error::{Error, Result, EXIT_PLAN_BLOCKED};
use migrator_core::gate::E2EBlockedReport;

use super::migrate;

/// Git + column change applied; caller runs plan/migrate; guard restores repo + scaffold on drop.
pub struct BlockedSetup {
    pub blocked_cfg: Config,
    guard: RestoreGuard,
}

pub async fn prepare_blocked_table_change(cfg: &Config) -> Result<BlockedSetup> {
    let sql_root = PathBuf::from(&cfg.sql_root);
    let table_sql = sql_root.join("dactests/smoke/tables/smoke_table.sql");
    let temp_repo = repo_root().join(".temp");

    cleanup_scaffold(&sql_root);

    migrate::run_apply_smoke(cfg).await?;

    let fp = format!("{}_{}", cfg.server, cfg.database);
    invalidate_audit_cache(&fp);
    let l1 = migrator_core::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
    let _ = l1.invalidate_all(&fp);

    restore_table_sql_from_git(&temp_repo, &table_sql)?;

    let original = std::fs::read_to_string(&table_sql).map_err(Error::Io)?;
    if original.contains("added_at") {
        return Err(Error::Other(anyhow::anyhow!(
            "smoke_table.sql already contains added_at after git restore"
        )));
    }
    let modified = original.replacen(
        "created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()",
        "created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),\n    added_at DATETIME2 NULL",
        1,
    );
    std::fs::write(&table_sql, &modified).map_err(Error::Io)?;

    let head = git_rev_parse(&temp_repo)?;
    let guard = RestoreGuard {
        temp_repo: temp_repo.clone(),
        sql_root: sql_root.clone(),
        head,
    };

    git_cmd(
        &temp_repo,
        &["add", "sql/dactests/smoke/tables/smoke_table.sql"],
    )?;
    git_cmd(
        &temp_repo,
        &["commit", "-m", "test: e2e add added_at column"],
    )?;

    let mut blocked_cfg = cfg.clone();
    blocked_cfg.session_socket.clear();
    blocked_cfg.skip_git = false;

    let fp = format!("{}_{}", blocked_cfg.server, blocked_cfg.database);
    invalidate_audit_cache(&fp);
    let l1 = migrator_core::cache::l1::L1Cache::new(&blocked_cfg.l1_cache_dir);
    let _ = l1.invalidate_all(&fp);

    Ok(BlockedSetup { blocked_cfg, guard })
}

pub async fn run_blocked_table_plan(cfg: &Config) -> Result<E2EBlockedReport> {
    let sql_root = PathBuf::from(&cfg.sql_root);
    let setup = prepare_blocked_table_change(cfg).await?;
    let blocked_cfg = setup.blocked_cfg;
    let _guard = setup.guard;

    let plan_out = run_command(Command::Plan, &blocked_cfg).await?;
    let plan = plan_out
        .plan
        .ok_or_else(|| Error::Other(anyhow::anyhow!("missing plan after column change")))?;
    if !plan.blocked {
        return Err(Error::Other(anyhow::anyhow!(
            "expected blocked plan after column change (actions={:?})",
            plan.objects
                .iter()
                .map(|o| (o.normalized_key.as_ref(), o.planned_action))
                .collect::<Vec<_>>()
        )));
    }

    let migrate_out = run_command(Command::Migrate, &blocked_cfg).await?;
    let exit_code = migrate_out.exit_code;
    let blocked = exit_code == EXIT_PLAN_BLOCKED;
    let scaffold_paths = list_scaffold_paths(&sql_root);

    Ok(E2EBlockedReport {
        scenario: "blocked_table_plan".into(),
        setup_steps: vec![
            "baseline_migrate".into(),
            "git_column_change".into(),
            "migrate_blocked".into(),
        ],
        exit_code,
        blocked,
        blockers: Vec::new(),
        scaffold_paths,
    })
}

struct RestoreGuard {
    temp_repo: PathBuf,
    sql_root: PathBuf,
    head: String,
}

impl Drop for RestoreGuard {
    fn drop(&mut self) {
        cleanup_scaffold(&self.sql_root);
        let _ = git_cmd(&self.temp_repo, &["reset", "--hard", &self.head]);
    }
}

fn repo_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../../..")
        .canonicalize()
        .expect("repo root")
}

fn restore_table_sql_from_git(temp_repo: &Path, dest: &Path) -> Result<()> {
    let out = OsCommand::new("git")
        .args([
            "-C",
            temp_repo.to_str().unwrap_or(""),
            "show",
            "HEAD:sql/dactests/smoke/tables/smoke_table.sql",
        ])
        .output()
        .map_err(Error::Io)?;
    if !out.status.success() {
        return Err(Error::Other(anyhow::anyhow!(
            "git show smoke_table.sql: {}",
            String::from_utf8_lossy(&out.stderr)
        )));
    }
    std::fs::write(dest, &out.stdout).map_err(Error::Io)
}

fn git_rev_parse(repo: &Path) -> Result<String> {
    let out = OsCommand::new("git")
        .args(["-C", repo.to_str().unwrap_or(""), "rev-parse", "HEAD"])
        .output()
        .map_err(Error::Io)?;
    if !out.status.success() {
        return Err(Error::Other(anyhow::anyhow!("git rev-parse failed")));
    }
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
}

fn git_cmd(repo: &Path, args: &[&str]) -> Result<()> {
    let mut cmd = OsCommand::new("git");
    cmd.arg("-C").arg(repo);
    for a in args {
        cmd.arg(a);
    }
    let out = cmd.output().map_err(Error::Io)?;
    if !out.status.success() {
        return Err(Error::Other(anyhow::anyhow!(
            "git {:?}: {}",
            args,
            String::from_utf8_lossy(&out.stderr)
        )));
    }
    Ok(())
}

fn list_scaffold_paths(sql_root: &Path) -> Vec<String> {
    let dir = sql_root.join("dactests/smoke/tables/_migrations/smoke_table");
    let Ok(entries) = std::fs::read_dir(&dir) else {
        return Vec::new();
    };
    entries
        .filter_map(|e| e.ok())
        .filter(|e| e.path().is_file())
        .map(|e| e.path().to_string_lossy().into_owned())
        .collect()
}

fn cleanup_scaffold(sql_root: &Path) {
    let dir = sql_root.join("dactests/smoke/tables/_migrations/smoke_table");
    if let Ok(entries) = std::fs::read_dir(&dir) {
        for e in entries.flatten() {
            if e.path().is_file() {
                let _ = std::fs::remove_file(e.path());
            }
        }
    }
    let _ = std::fs::remove_dir_all(&dir);
    let _ = std::fs::remove_dir(sql_root.join("dactests/smoke/tables/_migrations"));
}
