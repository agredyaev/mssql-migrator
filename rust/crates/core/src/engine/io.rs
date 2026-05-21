use crate::domain::Workspace;
use crate::error::Result;
use crate::export::{write_plan_json, MigrationPlan};
use crate::timings::PhaseTimings;

pub fn print_timings_json(timings: &PhaseTimings) -> Result<()> {
    let v =
        serde_json::to_string_pretty(timings).map_err(|e| crate::error::Error::Other(e.into()))?;
    println!("{v}");
    Ok(())
}

pub fn write_plan_stdout(plan: &MigrationPlan, ws: Option<&Workspace>) -> Result<()> {
    let mut stdout = std::io::stdout().lock();
    write_plan_json(plan, ws, &mut stdout)
}
