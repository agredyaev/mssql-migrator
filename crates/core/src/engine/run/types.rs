use crate::export::MigrationPlan;
use crate::timings::PhaseTimings;

/// Top-level command selected by the caller.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Command {
    /// Compute and display the migration plan without executing it.
    Plan,
    /// Execute pending migrations against the database.
    Migrate,
    /// Verify that all applied migrations match their recorded checksums.
    Validate,
    /// Record existing database objects as the checksum baseline without running them.
    Baseline,
    /// Recompute and store checksums for objects that have drifted.
    RepairChecksum,
    /// Print the tool version and exit.
    Version,
}

/// Output produced by a completed engine run.
pub struct RunOutput {
    /// Process exit code to be forwarded to the OS.
    pub exit_code: i32,
    /// Per-phase timing measurements collected during the run.
    pub timings: PhaseTimings,
    /// Migration plan, if one was produced (absent for `Version` and `Baseline` commands).
    pub plan: Option<MigrationPlan>,
}

pub(super) fn command_label(cmd: Command) -> &'static str {
    match cmd {
        Command::Plan => "plan",
        Command::Migrate => "migrate",
        Command::Validate => "validate",
        Command::Baseline => "baseline",
        Command::RepairChecksum => "repair-checksum",
        Command::Version => "version",
    }
}

/// Read-only commands must fail when the repository names databases that do
/// not exist on the server (silently skipping them validates nothing).
pub(super) fn missing_databases_error(dbs: &[String]) -> crate::error::Error {
    crate::error::Error::Config(format!(
        "catalog database(s) named by the repository do not exist on the server: {}",
        dbs.join(", ")
    ))
}
