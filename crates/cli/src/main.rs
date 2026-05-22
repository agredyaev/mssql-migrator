mod args;
mod help;

use std::path::Path;
use std::process::ExitCode;

use migrator_core::config::{build_config, load_env_file, validate_config};
use migrator_core::engine::{
    print_timings_json, print_version, run_command, write_plan_stdout, Command,
};
use migrator_core::export::write_reports;

use args::{parse_args, parse_command, ParsedArgs};
use help::print_help;

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
        } => run_command_line(&cmd, env_file, json).await,
    }
}

async fn run_command_line(
    cmd: &str,
    env_file: Option<String>,
    json: bool,
) -> migrator_core::Result<i32> {
    if cmd == "version" {
        print_version(json)?;
        return Ok(0);
    }
    let command = parse_command(cmd)?;
    let env_path = env_file.unwrap_or_else(|| ".env".into());
    let env = load_env_file(Path::new(&env_path))?;
    let mut cfg = build_config(&env, json);
    validate_config(&mut cfg)?;
    match run_command(command, &cfg).await {
        Ok(out) => {
            if json {
                print_timings_json(&out.timings)?;
            }
            if command == Command::Plan && json {
                if let Some(plan) = &out.plan {
                    write_plan_stdout(plan, None)?;
                }
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
