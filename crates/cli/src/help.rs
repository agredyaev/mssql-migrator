//! Help-text constants and printer for `rmig`.
//!
//! ### Purpose
//! Provides the `--help` text and short error hint used when an unknown flag
//! or command is encountered.

use std::io::Write;

/// Short hint printed when the user passes an unknown flag or command.
pub const HELP_HINT: &str = "Run 'rmig' or 'rmig --help' for usage.";

/// Full help text with usage, commands, flags, env vars, and examples.
pub const HELP: &str = "\
rmig — plan and apply MSSQL schema migrations from a repo SQL tree

Usage:
  rmig [--config <path>] [--json] <command>

Commands:
  plan              Scan SQL layout, diff against the database, write plan report
  migrate           Plan, scaffold blocked transitions, apply changes
  validate          Run checks without applying
  baseline          Adopt repository objects already present in the DB (initial baseline)
  repair-checksum   Repair audit checksum mismatches
  version           Print release version and git commit

Flags:
  --config <path>   Load typed TOML settings (default: config.toml)
  --json            Emit JSON output; with plan, write plan JSON to stdout and timings to stderr
  -h, --help        Show this help

Setup:
  1. Create config.toml in the working directory (or pass --config).
  2. Set database.server and paths.sql_root; set RM_DB_USER and RM_DB_PASSWORD in the environment.
  3. Run: rmig plan          # preview changes
     Run: rmig migrate        # apply when the plan looks correct

TOML fields and environment overrides:
  database.server / RM_DB_SERVER     SQL Server host (required)
  paths.sql_root / RM_SQL_ROOT       Root of <database>/<schema>/<kind>/<object>.sql tree (required)
  RM_DB_USER / RM_DB_PASSWORD        SQL authentication (required; environment only)
  paths.sql_base / RM_SQL_BASE       Migrations/scaffold path (defaults to RM_SQL_ROOT)
  paths.report_dir / RM_REPORT_DIR   Directory for .plan.json / .report.json artifacts
  RM_SKIP_GIT=1             Full catalog inspect without git delta preload
  RMIG_SESSION              Unix socket path when using rmigd session daemon

Examples:
  rmig plan
  rmig --config ops/config.toml plan --json
  rmig migrate
  rmig version
";

/// Print full help text and usage examples to the given writer.
pub fn print_help(mut w: impl Write) -> migrator_core::Result<()> {
    write!(w, "{HELP}")?;
    Ok(())
}
