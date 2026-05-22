use crate::export::MigrationPlan;
use crate::timings::PhaseTimings;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Command {
    Plan,
    Migrate,
    Validate,
    Baseline,
    RepairChecksum,
    Version,
}

pub struct RunOutput {
    pub exit_code: i32,
    pub timings: PhaseTimings,
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
