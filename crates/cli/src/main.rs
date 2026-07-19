//! `rmig` — CLI entry point for MSSQL schema migration operations.
//!
//! ### Purpose
//! Parses argv, loads typed TOML plus environment secrets, builds config, dispatches to
//! [`migrator_core::engine::run_command`], and writes JSON/report output.
//!
//! ### Subcommands (delegated to `args`/`help` modules)
//! - `plan`         — scan SQL layout, diff against DB, write plan report
//! - `migrate`      — plan + scaffold + apply
//! - `validate`     — checks without applying
//! - `baseline`     — adopt repository objects already present in the DB (baseline)
//! - `repair-checksum` — fix audit checksum mismatches
//! - `version`      — print release + git commit
//!
//! ### Environment
//! - `--config <path>` or `config.toml` — typed non-secret settings
//! - `RM_*` vars — see [`migrator_core::config`]
//!
//! ### Exit codes
//! - 0 — success
//! - non-zero — see [`migrator_core::error::Error::exit_code`]

#![forbid(unsafe_code)]
mod args;
mod help;
mod logging;
mod signals;

use std::process::ExitCode;

use migrator_core::config::{
    build_config, load_toml_config, load_toml_config_required, validate_config,
};
use migrator_core::engine::{
    print_timings_json, print_version, run_command, write_plan_stdout, Command,
};
use migrator_core::export::write_reports;

use args::{parse_args, parse_command, ParsedArgs};
use help::print_help;
use logging::init_tracing;
use signals::shutdown_signal;

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
            config_file,
            json,
        } => run_command_line(&cmd, config_file.as_deref(), json).await,
    }
}

async fn run_command_line(
    cmd: &str,
    config_file: Option<&str>,
    json: bool,
) -> migrator_core::Result<i32> {
    if cmd == "version" {
        print_version(json)?;
        return Ok(0);
    }
    let command = parse_command(cmd)?;
    let file = match config_file {
        Some(path) => load_toml_config_required(std::path::Path::new(path))?,
        None => load_toml_config(std::path::Path::new("config.toml"))?,
    };
    let mut cfg = build_config(&file, json);
    init_tracing(&cfg.log_level);
    validate_config(&mut cfg)?;
    let outcome = tokio::select! {
        out = run_command(command, &cfg) => out,
        sig = shutdown_signal() => {
            // Dropping the run future closes the TDS connection: the server
            // rolls back any open transaction and frees the session applock.
            eprintln!("rmig: interrupted by {sig}; aborted at a safe statement boundary");
            let _ = write_reports(&cfg, cmd, None, None, 130);
            return Ok(130);
        }
    };
    match outcome {
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
            if out.exit_code == migrator_core::error::EXIT_PLAN_BLOCKED && !json {
                // Blocked plans exit 10 through the Ok path; without this the
                // console shows nothing and the blockers exist only in --json.
                if let Some(plan) = &out.plan {
                    for b in &plan.blockers {
                        eprintln!("rmig: blocked: {b}");
                    }
                }
                eprintln!("rmig: plan is blocked; run 'rmig plan --json' for details");
            }
            write_reports(&cfg, cmd, out.plan.as_ref(), None, out.exit_code)?;
            Ok(out.exit_code)
        }
        Err(e) => {
            let code = e.exit_code();
            if let Err(werr) = write_reports(&cfg, cmd, None, None, code) {
                eprintln!("warning: failed to write failure report: {werr}");
            }
            Err(e)
        }
    }
}
