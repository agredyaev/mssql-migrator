//! `rmig` — CLI entry point for MSSQL schema migration operations.
//!
//! ### Purpose
//! Parses argv, loads environment (dotenv), builds config, dispatches to
//! [`migrator_core::engine::run_command`], and writes JSON/report output.
//!
//! ### Subcommands (delegated to `args`/`help` modules)
//! - `plan`         — scan SQL layout, diff against DB, write plan report
//! - `migrate`      — plan + scaffold + apply
//! - `validate`     — checks without applying
//! - `baseline`     — adopt all catalog objects as baseline
//! - `repair-checksum` — fix audit checksum mismatches
//! - `version`      — print release + git commit
//!
//! ### Environment
//! - `--env <path>` or `.env` — loaded via [`load_env_file_required`] / [`load_env_file`]
//! - `RM_*` vars — see [`migrator_core::config`]
//!
//! ### Exit codes
//! - 0 — success
//! - non-zero — see [`migrator_core::error::Error::exit_code`]

mod args;
mod help;
mod logging;

use std::path::Path;
use std::process::ExitCode;

use migrator_core::config::{build_config, load_env_file, load_env_file_required, validate_config};
use migrator_core::engine::{
    print_timings_json, print_version, run_command, write_plan_stdout, Command,
};
use migrator_core::export::write_reports;

use args::{parse_args, parse_command, ParsedArgs};
use help::print_help;
use logging::init_tracing;

#[tokio::main]
async fn main() -> ExitCode {
    match run(std::env::args().skip(1).collect()).await {
        Ok(code) => ExitCode::from(code as u8),
        Err(e) => {
            eprintln!("rmig: {e}");
            ExitCode::from(e.exit_code() as u8)
        }
    }
}

async fn run(args: Vec<String>) -> migrator_core::Result<i32> {
    match parse_args(&args)? {
        ParsedArgs::Help => {
            print_help(std::io::stdout())?;
            Ok(0)
        }
        ParsedArgs::Run {
            cmd,
            env_file,
            json,
        } => run_command_line(&cmd, env_file.as_deref(), json).await,
    }
}

async fn run_command_line(
    cmd: &str,
    env_file: Option<&str>,
    json: bool,
) -> migrator_core::Result<i32> {
    if cmd == "version" {
        print_version(json)?;
        return Ok(0);
    }
    let command = parse_command(cmd)?;
    let env = match env_file {
        Some(path) => load_env_file_required(Path::new(path))?,
        None => load_env_file(Path::new(".env"))?,
    };
    let mut cfg = build_config(&env, json);
    init_tracing(&cfg.log_level);
    validate_config(&mut cfg)?;
    match run_command(command, &cfg).await {
        Ok(out) => {
            if command == Command::Plan && json {
                if let Some(plan) = &out.plan {
                    write_plan_stdout(plan, None)?;
                }
                eprintln!(
                    "{}",
                    serde_json::to_string_pretty(&out.timings)
                        .map_err(|e| migrator_core::Error::Other(e.into()))?
                );
            } else if json {
                print_timings_json(&out.timings)?;
            }
            write_reports(&cfg, cmd, out.plan.as_ref(), None, out.exit_code)?;
            Ok(out.exit_code)
        }
        Err(e) => {
            let code = e.exit_code();
            let _ = write_reports(&cfg, cmd, None, None, code);
            Err(e)
        }
    }
}
