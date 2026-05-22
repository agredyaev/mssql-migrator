use std::io::Write;

pub const HELP_HINT: &str = "Run 'rmig' or 'rmig --help' for usage.";

pub const HELP: &str = "\
rmig — plan and apply MSSQL schema migrations from a repo SQL tree

Usage:
  rmig [--env <path>] [--json] <command>

Commands:
  plan              Scan SQL layout, diff against the database, write plan report
  migrate           Plan, scaffold blocked transitions, apply changes
  validate          Run checks without applying
  baseline          Mark all catalog objects as adopted (initial baseline)
  repair-checksum   Repair audit checksum mismatches
  version           Print release version and git commit

Flags:
  --env <path>      Load RM_* settings from a dotenv file (default: .env)
  --json            Emit phase timings as JSON; with plan, write plan JSON to stdout
  -h, --help        Show this help

Setup:
  1. Create .env in the working directory (or pass --env).
  2. Set at least RM_DB_SERVER, RM_SQL_ROOT, RM_DB_USER, RM_DB_PASSWORD.
  3. Run: rmig plan          # preview changes
     Run: rmig migrate        # apply when the plan looks correct

Common environment variables (.env):
  RM_DB_SERVER              SQL Server host (required)
  RM_SQL_ROOT               Root of <database>/<schema>/<kind>/<object>.sql tree (required)
  RM_DB_USER / RM_DB_PASSWORD   SQL authentication (required unless integrated auth)
  RM_SQL_BASE               Migrations/scaffold path (defaults to RM_SQL_ROOT)
  RM_REPORT_DIR             Directory for .plan.json / .report.json artifacts
  RM_SKIP_GIT=1             Full catalog inspect without git delta preload
  RMIG_SESSION              Unix socket path when using rmigd session daemon

Examples:
  rmig plan
  rmig --env ops/.env plan --json
  rmig migrate
  rmig version
";

pub fn print_help(mut w: impl Write) -> migrator_core::Result<()> {
    write!(w, "{HELP}")?;
    Ok(())
}
