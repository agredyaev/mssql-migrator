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
    if let Some(plan) = plan {
        let path = dir.join(".plan.json");
        let mut f = File::create(&path)?;
        write_plan_json(plan, ws, &mut f)?;
    }
    let result = if exit_code == 0 { "success" } else { "failure" };
    let report = RunFinished {
        command: command.into(),
        result: result.into(),
        exit_code,
    };
    write_json_file(&dir.join(".report.json"), &report, cfg.report_sync())?;
    Ok(())
}

fn write_json_file(path: &Path, v: &impl Serialize, sync: bool) -> Result<()> {
    let f = File::create(path)?;
    let mut w = BufWriter::new(f);
    serde_json::to_writer_pretty(&mut w, v).map_err(|e| Error::Other(e.into()))?;
    w.write_all(b"\n").map_err(Error::Io)?;
    w.flush().map_err(Error::Io)?;
    if sync {
        w.get_ref().sync_all().map_err(Error::Io)?;
    }
    Ok(())
}
