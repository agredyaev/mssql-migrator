//! Go↔Rust plan snapshot parity on `.temp/sql` + Docker SQL Server.
//!
//! Run: `make go-rust-e2e`

mod common;

use migrator_core::engine::{run_command, Command};
use migrator_core::gate::{parity_messages, read_snapshot_json, PlanSnapshot};

#[tokio::test]
async fn go_rust_plan_matches_go_reference() {
    if !common::integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let go_path = match std::env::var("RMIG_E2E_GO_SNAPSHOT") {
        Ok(p) if !p.is_empty() => std::path::PathBuf::from(p),
        _ => {
            eprintln!("skip: set RMIG_E2E_GO_SNAPSHOT to Go-exported snapshot path");
            return;
        }
    };

    let go_data =
        std::fs::read_to_string(&go_path).unwrap_or_else(|e| panic!("read go snapshot: {e}"));
    let go_snap: PlanSnapshot = read_snapshot_json(&go_data).expect("parse go snapshot");

    let cfg = common::config();
    let out = run_command(Command::Plan, cfg).await.expect("rust plan");
    let plan = out.plan.expect("migration plan");
    let rust_snap = PlanSnapshot::from_plan(&plan);

    let diffs = parity_messages(&go_snap, &rust_snap);

    if let Ok(out_path) = std::env::var("RMIG_E2E_RUST_SNAPSHOT") {
        if !out_path.is_empty() {
            migrator_core::gate::write_snapshot_file(std::path::Path::new(&out_path), &rust_snap)
                .expect("write rust snapshot");
        }
    }

    if !diffs.is_empty() {
        for d in &diffs {
            eprintln!("parity: {d}");
        }
        panic!("Go↔Rust plan snapshot mismatch ({} diffs)", diffs.len());
    }
    eprintln!(
        "Go↔Rust parity OK: {} objects on .temp/sql",
        go_snap.objects.len()
    );
}
