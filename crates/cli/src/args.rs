//! CLI argument parsing for `rmig`.
//!
//! ### Purpose
//! Tokenises argv into [`ParsedArgs`] (help or run) and validates subcommand
//! names via [`parse_command`].
//!
//! ### Flags
//! - `--config <path>` — typed TOML config path (default `config.toml`)
//! - `--json` — JSON output mode
//! - `-h` / `--help` — print help text and exit
//!
//! ### Nominal flow
//! 1. `parse_args` walks argv, classifies flags vs positional command.
//! 2. If no command given → returns `ParsedArgs::Help`.
//! 3. `parse_command` maps string to [`Command`] enum.

use migrator_core::engine::Command;
use migrator_core::error::Error;

/// Parsed CLI invocation: either a help request or a run command with flags.
pub enum ParsedArgs {
    /// User requested `--help`.
    Help,
    /// User requested a command with optional flags.
    Run {
        /// Subcommand string (`plan`, `migrate`, …).
        cmd: String,
        /// Optional `--config <path>` value.
        config_file: Option<String>,
        /// `--json` flag.
        json: bool,
    },
}

/// Tokenise argv into [`ParsedArgs`].
pub fn parse_args(args: &[String]) -> migrator_core::Result<ParsedArgs> {
    let mut cmd = String::new();
    let mut config_file = None;
    let mut json = false;
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--config" => {
                i += 1;
                // A following flag is NOT a path: swallowing it would both
                // lose that flag and skip the missing-value diagnostic.
                let val = args.get(i).filter(|v| !v.starts_with('-')).ok_or_else(|| {
                    Error::InvalidInput(format!(
                        "--config requires a path argument\n\n{}",
                        super::help::HELP_HINT
                    ))
                })?;
                config_file = Some(val.clone());
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
        config_file,
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

#[cfg(test)]
#[path = "tests/args_test.rs"]
mod args_tests;
