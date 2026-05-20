use std::path::Path;
use std::process::ExitCode;

use migrator_core::config::{build_config, load_env_file, validate_config};
use migrator_core::engine::{print_timings_json, run_command, write_plan_stdout, Command};
use migrator_core::error::Error;
use migrator_core::export::write_reports;

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
    let (cmd, env_file, json) = parse_args(&args)?;
    if cmd == "version" {
        if json {
            println!(r#"{{"version":"0.1.0","commit":"rust-port"}}"#);
        } else {
            println!("rmig 0.1.0 (rust-port)");
        }
        return Ok(0);
    }
    let env_path = env_file.unwrap_or_else(|| ".env".into());
    let env = load_env_file(Path::new(&env_path))?;
    let mut cfg = build_config(&env, json);
    validate_config(&mut cfg)?;
    let command = match cmd.as_str() {
        "plan" => Command::Plan,
        "migrate" => Command::Migrate,
        "validate" => Command::Validate,
        "baseline" => Command::Baseline,
        "repair-checksum" => Command::RepairChecksum,
        _ => return Err(Error::InvalidInput(format!("unknown command: {cmd}"))),
    };
    match run_command(command, &cfg).await {
        Ok(out) => {
            if json {
                print_timings_json(&out.timings)?;
            }
            if command == Command::Plan && json {
                if let Some(plan) = &out.plan {
                    write_plan_stdout(plan)?;
                }
            }
            write_reports(&cfg, &cmd, out.plan.as_ref(), out.exit_code)?;
            Ok(out.exit_code)
        }
        Err(e) => {
            let code = e.exit_code();
            let _ = write_reports(&cfg, &cmd, None, code);
            Err(e)
        }
    }
}

fn parse_args(args: &[String]) -> migrator_core::Result<(String, Option<String>, bool)> {
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
            "-h" | "--help" => {
                return Err(Error::InvalidInput(USAGE.into()));
            }
            other if other.starts_with('-') => {
                return Err(Error::InvalidInput(format!("unknown flag: {other}")));
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
        return Err(Error::InvalidInput(format!("missing command\n{USAGE}")));
    }
    Ok((cmd, env_file, json))
}

const USAGE: &str =
    "Usage: rmig [--env <path>] [--json] <plan|migrate|validate|baseline|repair-checksum|version>";
