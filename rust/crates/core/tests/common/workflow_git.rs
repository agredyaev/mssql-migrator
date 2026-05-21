//! Git fixture edits under `.temp/` for workflow integration.

use std::path::{Path, PathBuf};
use std::process::Command as OsCommand;

use migrator_core::audit::invalidate_audit_cache;
use migrator_core::error::Result;

use super::workflow_config;

pub fn git_repo() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../.temp")
        .canonicalize()
        .unwrap_or_else(|_| PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../../.temp"))
}

pub fn sql_path(rel: &str) -> PathBuf {
    PathBuf::from(&workflow_config::workflow_config().sql_root).join(rel)
}

pub struct GitRestore {
    repo: PathBuf,
    head: String,
}

impl GitRestore {
    pub fn open() -> Result<Self> {
        let repo = git_repo();
        let head = git_rev_parse(&repo)?;
        Ok(Self { repo, head })
    }

    pub fn write_and_commit(&self, rel_from_sql: &str, content: &str, message: &str) -> Result<()> {
        std::fs::write(sql_path(rel_from_sql), content).map_err(migrator_core::error::Error::Io)?;
        git_cmd(&self.repo, &["add", &format!("sql/{rel_from_sql}")])?;
        git_cmd(&self.repo, &["commit", "-m", message])?;
        invalidate_caches();
        Ok(())
    }

    pub fn add_tree_and_commit(&self, rel_from_sql: &str, message: &str) -> Result<()> {
        git_cmd(&self.repo, &["add", "-A", &format!("sql/{rel_from_sql}")])?;
        git_cmd(&self.repo, &["commit", "-m", message])?;
        invalidate_caches();
        Ok(())
    }
}

impl Drop for GitRestore {
    fn drop(&mut self) {
        let _ = git_cmd(&self.repo, &["reset", "--hard", &self.head]);
        cleanup_smoke_scaffold();
    }
}

fn invalidate_caches() {
    let cfg = workflow_config::workflow_config();
    let fp = format!("{}_{}", cfg.server, cfg.database);
    invalidate_audit_cache(&fp);
}

fn cleanup_smoke_scaffold() {
    let sql_root = PathBuf::from(&workflow_config::workflow_config().sql_root);
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

fn git_rev_parse(repo: &Path) -> Result<String> {
    let out = OsCommand::new("git")
        .args(["-C", repo.to_str().unwrap_or(""), "rev-parse", "HEAD"])
        .output()
        .map_err(migrator_core::error::Error::Io)?;
    if !out.status.success() {
        return Err(migrator_core::error::Error::Other(anyhow::anyhow!(
            "git rev-parse failed"
        )));
    }
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
}

fn git_cmd(repo: &Path, args: &[&str]) -> Result<()> {
    let mut cmd = OsCommand::new("git");
    cmd.arg("-C").arg(repo);
    for a in args {
        cmd.arg(a);
    }
    let out = cmd.output().map_err(migrator_core::error::Error::Io)?;
    if !out.status.success() {
        return Err(migrator_core::error::Error::Other(anyhow::anyhow!(
            "git {:?}: {}",
            args,
            String::from_utf8_lossy(&out.stderr)
        )));
    }
    Ok(())
}
