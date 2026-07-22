use crate::domain::Workspace;
use crate::error::Result;
use crate::export::{write_plan_json, MigrationPlan};
use crate::timings::PhaseTimings;

/// Prints `timings` as pretty-printed JSON to stdout, or to stderr when
/// `to_stderr` (used when stdout already carries the plan JSON).
pub fn write_timings_json(timings: &PhaseTimings, to_stderr: bool) -> Result<()> {
    let v =
        serde_json::to_string_pretty(timings).map_err(|e| crate::error::Error::Other(e.into()))?;
    if to_stderr {
        eprintln!("{v}");
    } else {
        println!("{v}");
    }
    Ok(())
}

/// Prints build version information to stdout.
pub fn print_version(json: bool) -> Result<()> {
    if json {
        crate::buildinfo::write_json(std::io::stdout())
    } else {
        println!("{}", crate::buildinfo::summary());
        Ok(())
    }
}

/// Writes `plan` as JSON to stdout.
pub fn write_plan_stdout(plan: &MigrationPlan, ws: Option<&Workspace>) -> Result<()> {
    let mut stdout = std::io::stdout().lock();
    write_plan_json(plan, ws, &mut stdout)
}
