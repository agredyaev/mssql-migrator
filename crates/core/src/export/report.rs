use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::Path;

use serde::Serialize;

use super::plan_json::write_plan_json;
use super::MigrationPlan;
use crate::config::Config;
use crate::domain::Workspace;
use crate::error::{Error, Result};

/// Summary record written to the run report file.
#[derive(Debug, Serialize)]
pub struct RunFinished {
    /// Name of the command that was executed.
    pub command: String,
    /// Human-readable outcome label ("success" or "failure").
    pub result: String,
    /// Process exit code produced by the run.
    #[serde(rename = "exitCode")]
    pub exit_code: i32,
}

/// Writes plan and run-finished report files to the configured report directory.
pub fn write_reports(
    cfg: &Config,
    command: &str,
    plan: Option<&MigrationPlan>,
    ws: Option<&Workspace>,
    exit_code: i32,
) -> Result<()> {
    if cfg.report_dir.is_empty() {
        return Ok(());
    }
    let dir = Path::new(&cfg.report_dir);
    fs::create_dir_all(dir)?;
    let plan_path = dir.join(".plan.json");
    if let Some(plan) = plan {
        write_atomic(&plan_path, |f| write_plan_json(plan, ws, f))?;
    } else if let Err(e) = fs::remove_file(&plan_path) {
        // A failed run must not leave the previous run's plan next to its own
        // failure report — operators would diagnose against the wrong plan.
        if e.kind() != std::io::ErrorKind::NotFound {
            return Err(Error::Io(e));
        }
    }
    let result = if exit_code == 0 { "success" } else { "failure" };
    let report = RunFinished {
        command: command.into(),
        result: result.into(),
        exit_code,
    };
    let sync = cfg.report_sync;
    write_atomic(&dir.join(".report.json"), |f| write_json(f, &report, sync))?;
    Ok(())
}

/// Writes via a UNIQUE sibling temp file then renames over `path`, so a crash
/// or concurrent reader never observes a truncated report and two concurrent
/// writers never race on one temp pathname (last complete writer wins).
fn write_atomic(path: &Path, write: impl FnOnce(&mut File) -> Result<()>) -> Result<()> {
    use std::sync::atomic::{AtomicU64, Ordering};
    static SEQ: AtomicU64 = AtomicU64::new(0);
    let tmp = path.with_extension(format!(
        "tmp.{}.{}",
        std::process::id(),
        SEQ.fetch_add(1, Ordering::Relaxed)
    ));
    {
        let mut f = File::create(&tmp)?;
        write(&mut f)?;
        f.sync_all().map_err(Error::Io)?;
    }
    fs::rename(&tmp, path).map_err(Error::Io)?;
    Ok(())
}

fn write_json(f: &mut File, v: &impl Serialize, sync: bool) -> Result<()> {
    let mut w = BufWriter::new(f);
    serde_json::to_writer_pretty(&mut w, v).map_err(|e| Error::Other(e.into()))?;
    w.write_all(b"\n").map_err(Error::Io)?;
    w.flush().map_err(Error::Io)?;
    if sync {
        w.get_ref().sync_all().map_err(Error::Io)?;
    }
    Ok(())
}
