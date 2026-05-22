use migrator_core::engine::Command;
use migrator_core::error::Error;

pub enum ParsedArgs {
    Help,
    Run {
        cmd: String,
        env_file: Option<String>,
        json: bool,
    },
}

pub fn parse_args(args: &[String]) -> migrator_core::Result<ParsedArgs> {
    let mut cmd = String::new();
    let mut env_file = None;
    let mut json = false;
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--env" => {
                i += 1;
                env_file = Some(
                    args.get(i)
                        .ok_or_else(|| Error::InvalidInput("--env requires path".into()))?
                        .clone(),
                );
            }
            "--json" => json = true,
            "-h" | "--help" => return Ok(ParsedArgs::Help),
            other if other.starts_with('-') => {
                return Err(Error::InvalidInput(format!(
                    "unknown flag: {other}\n\n{}",
                    super::help::HELP_HINT
                )));
            }
            other => {
                if !cmd.is_empty() {
                    return Err(Error::InvalidInput(format!("unexpected arg: {other}")));
                }
                cmd = other.into();
            }
        }
        i += 1;
    }
    if cmd.is_empty() {
        return Ok(ParsedArgs::Help);
    }
    Ok(ParsedArgs::Run {
        cmd,
        env_file,
        json,
    })
}

pub fn parse_command(cmd: &str) -> migrator_core::Result<Command> {
    match cmd {
        "plan" => Ok(Command::Plan),
        "migrate" => Ok(Command::Migrate),
        "validate" => Ok(Command::Validate),
        "baseline" => Ok(Command::Baseline),
        "repair-checksum" => Ok(Command::RepairChecksum),
        other => Err(Error::InvalidInput(format!(
            "unknown command: {other}\n\n{}",
            super::help::HELP_HINT
        ))),
    }
}
