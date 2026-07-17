//! Blocked migrate + scaffold e2e (blocked_table_plan scenario).

use std::path::{Path, PathBuf};
use std::process::Command as OsCommand;
use std::time::Instant;

use migrator_core::audit::invalidate_audit_cache;
use migrator_core::config::Config;
use migrator_core::engine::{run_command, Command};
use migrator_core::error::{Error, Result, EXIT_PLAN_BLOCKED};
use migrator_core::gate::{E2EBlockedReport, E2EWorkflowTimings};

use super::catalog;
use super::e2e_verify;
use super::migrate;
use crate::db_reset_skip;

/// Git + column change applied; caller runs plan/migrate; guard restores repo + scaffold on drop.
pub struct BlockedSetup {
    pub blocked_cfg: Config,
    guard: RestoreGuard,
}

pub async fn prepare_blocked_table_change(cfg: &Config) -> Result<(BlockedSetup, i64)> {
    let sql_root = PathBuf::from(&cfg.sql_root);
    let catalog_db = catalog::sole_catalog_database(&cfg.sql_root)?;
    let table_rel = catalog::catalog_sql_rel(&cfg.sql_root, "smoke/tables/smoke_table.sql")?;
    let table_sql = sql_root.join(&table_rel);
    let temp_repo = repo_root().join(".temp");

    cleanup_scaffold(&sql_root, &catalog_db);

    let setup_apply_ms = if db_reset_skip::skip_db_reset() {
        // e2e orchestrator already ran apply_smoke_setup / prior scenarios on this DB.
        0
    } else {
        let t = Instant::now();
        migrate::ensure_smoke_baseline(cfg).await?;
        migrator_core::timings::dur_ms(t.elapsed())
    };

    restore_table_sql_from_git(&temp_repo, &table_rel, &table_sql)?;

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
        database: catalog_db.clone(),
        head,
    };

    git_cmd(&temp_repo, &["add", &format!("sql/{table_rel}")])?;
    git_cmd(
        &temp_repo,
        &["commit", "-m", "test: e2e add added_at column"],
    )?;

    let mut blocked_cfg = cfg.clone();
    blocked_cfg.session_socket.clear();
    blocked_cfg.set_skip_git(false);

    let fp = migrator_core::audit::db_fingerprint(
        &blocked_cfg.server,
        &blocked_cfg.port,
        &blocked_cfg.user,
        &blocked_cfg.database,
    );
    invalidate_audit_cache(&fp);
    let l1 = migrator_core::cache::l1::L1Cache::new(&blocked_cfg.l1_cache_dir);
    let _ = l1.invalidate_all(&fp);

    Ok((BlockedSetup { blocked_cfg, guard }, setup_apply_ms))
}

fn workflow_timings_from_runs(
    setup_apply_ms: i64,
    plan: &migrator_core::timings::PhaseTimings,
    migrate: &migrator_core::timings::PhaseTimings,
    total_ms: i64,
) -> E2EWorkflowTimings {
    let plan_db_path = if !plan.plan_db_path.is_empty() {
        plan.plan_db_path.clone()
    } else {
        migrate.plan_db_path.clone()
    };
    E2EWorkflowTimings {
        setup_apply_ms,
        plan_wall_ms: plan.plan_wall_ms,
        plan_parallel_wall_ms: plan.parallel_wall_ms,
        plan_db_path,
        migrate_wall_ms: migrate.plan_wall_ms,
        migrate_parallel_wall_ms: migrate.parallel_wall_ms,
        total_ms,
    }
}

pub async fn run_blocked_table_plan(cfg: &Config) -> Result<E2EBlockedReport> {
    let t_all = Instant::now();
    let sql_root = PathBuf::from(&cfg.sql_root);
    let (setup, setup_apply_ms) = prepare_blocked_table_change(cfg).await?;
    let blocked_cfg = setup.blocked_cfg;
    let _guard = setup.guard;

    let migrate_out = run_command(Command::Migrate, &blocked_cfg).await?;
    let plan = migrate_out
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
    let exit_code = migrate_out.exit_code;
    if exit_code != EXIT_PLAN_BLOCKED {
        return Err(Error::Other(anyhow::anyhow!(
            "expected blocked migrate exit {EXIT_PLAN_BLOCKED}, got {exit_code}"
        )));
    }
    let blocked = true;

    use migrator_core::driver::{connect, DbClient, IoProfile, TimingConn};
    use std::sync::{Arc, Mutex};
    let io_arc = Arc::new(Mutex::new(IoProfile::default()));
    let mut conn = TimingConn::new(
        DbClient::Direct(connect(&blocked_cfg).await?.client),
        io_arc,
        0,
    );
    e2e_verify::verify_blocked_after_migrate(&mut conn, &sql_root).await?;

    let scaffold_paths =
        list_scaffold_paths(&sql_root, &catalog::sole_catalog_database(&cfg.sql_root)?);
    if blocked && scaffold_paths.is_empty() {
        return Err(Error::Other(anyhow::anyhow!(
            "expected scaffold files after blocked migrate"
        )));
    }

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
        timings: workflow_timings_from_runs(
            setup_apply_ms,
            &migrate_out.timings,
            &migrate_out.timings,
            migrator_core::timings::dur_ms(t_all.elapsed()),
        ),
    })
}

/// Blocked DDL → commit transition → successful migrate; verifies `history.kind = migration`.
pub async fn run_ddl_transition_apply(cfg: &Config) -> Result<migrator_core::gate::E2EApplyReport> {
    use migrator_core::driver::{connect, DbClient, IoProfile, TimingConn};
    use migrator_core::gate::E2EApplyReport;
    use std::sync::{Arc, Mutex};

    let t_all = Instant::now();
    let (setup, setup_apply_ms) = prepare_blocked_table_change(cfg).await?;
    let blocked_cfg = setup.blocked_cfg;
    let guard = setup.guard;

    let blocked = run_command(Command::Migrate, &blocked_cfg).await?;
    if blocked.exit_code != EXIT_PLAN_BLOCKED {
        return Err(Error::Other(anyhow::anyhow!(
            "expected blocked migrate exit {EXIT_PLAN_BLOCKED}, got {}",
            blocked.exit_code
        )));
    }

    let sql_root = PathBuf::from(&cfg.sql_root);
    let io_arc = Arc::new(Mutex::new(IoProfile::default()));
    let mut conn = TimingConn::new(
        DbClient::Direct(connect(&blocked_cfg).await?.client),
        io_arc,
        0,
    );
    e2e_verify::verify_blocked_after_migrate(&mut conn, &sql_root).await?;

    let migration_dir =
        catalog::catalog_sql_rel(&cfg.sql_root, "smoke/tables/_migrations/smoke_table")?;
    let temp_repo = repo_root().join(".temp");
    git_cmd(&temp_repo, &["add", "-A", &format!("sql/{migration_dir}")])?;
    git_cmd(
        &temp_repo,
        &["commit", "-m", "test: e2e track transition migration"],
    )?;

    let mig_before = super::migrate::count_audit_rows(&mut conn, "migration").await?;
    let apply = run_command(Command::Migrate, &blocked_cfg).await?;
    if apply.exit_code != 0 {
        return Err(Error::Other(anyhow::anyhow!(
            "transition migrate failed: exit {}",
            apply.exit_code
        )));
    }

    let snap =
        e2e_verify::verify_ddl_transition_applied(&blocked_cfg, &mut conn, &sql_root).await?;
    // Counters derive from the observed run: applied = new migration audit
    // rows; skipped from the plan summary; failed guarded by exit_code above.
    let applied = snap.audit_migration_rows - mig_before;
    let skipped = apply
        .plan
        .as_ref()
        .map(|p| p.summary.skip_count as i32)
        .unwrap_or(0);

    drop(guard);

    Ok(E2EApplyReport {
        scenario: "ddl_transition_apply".into(),
        setup_steps: vec![
            "baseline_migrate".into(),
            "git_column_change".into(),
            "migrate_blocked".into(),
            "commit_transition".into(),
            "migrate_transition".into(),
        ],
        applied,
        failed: 0,
        skipped,
        errors: Vec::new(),
        audit_object_rows: snap.audit_object_rows,
        audit_migration_rows: snap.audit_migration_rows,
        catalog_meta_rows: snap.catalog_meta_rows,
        catalog_cache_rows: snap.catalog_cache_rows,
        timings: E2EWorkflowTimings {
            setup_apply_ms,
            plan_wall_ms: blocked.timings.plan_wall_ms,
            plan_parallel_wall_ms: blocked.timings.parallel_wall_ms,
            plan_db_path: blocked.timings.plan_db_path.clone(),
            migrate_wall_ms: apply.timings.plan_wall_ms,
            migrate_parallel_wall_ms: apply.timings.parallel_wall_ms,
            total_ms: migrator_core::timings::dur_ms(t_all.elapsed()),
        },
    })
}

struct RestoreGuard {
    temp_repo: PathBuf,
    sql_root: PathBuf,
    database: String,
    head: String,
}

impl Drop for RestoreGuard {
    fn drop(&mut self) {
        cleanup_scaffold(&self.sql_root, &self.database);
        let _ = git_cmd(&self.temp_repo, &["reset", "--hard", &self.head]);
    }
}

fn restore_table_sql_from_git(temp_repo: &Path, table_rel: &str, dest: &Path) -> Result<()> {
    let git_path = format!("HEAD:sql/{table_rel}");
    let out = OsCommand::new("git")
        .args(["-C", temp_repo.to_str().unwrap_or(""), "show", &git_path])
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

fn migrations_dir(sql_root: &Path, database: &str) -> PathBuf {
    sql_root
        .join(database)
        .join("smoke/tables/_migrations/smoke_table")
}

fn repo_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
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

fn list_scaffold_paths(sql_root: &Path, database: &str) -> Vec<String> {
    let dir = migrations_dir(sql_root, database);
    let Ok(entries) = std::fs::read_dir(&dir) else {
        return Vec::new();
    };
    entries
        .filter_map(|e| e.ok())
        .filter(|e| e.path().is_file())
        .map(|e| e.path().to_string_lossy().into_owned())
        .collect()
}

fn cleanup_scaffold(sql_root: &Path, database: &str) {
    let dir = migrations_dir(sql_root, database);
    if let Ok(entries) = std::fs::read_dir(&dir) {
        for e in entries.flatten() {
            if e.path().is_file() {
                let _ = std::fs::remove_file(e.path());
            }
        }
    }
    let _ = std::fs::remove_dir_all(&dir);
    let _ = std::fs::remove_dir(sql_root.join(database).join("smoke/tables/_migrations"));
}
