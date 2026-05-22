//! Prod incremental go/no-go gate (SQL Server).
//!
//! Run: `make prod-gate` or `RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test --test prod_gate_integration -- --nocapture`

mod common;

#[path = "common/gate_paths.rs"]
mod gate_paths;

#[path = "common/gate.rs"]
mod gate;

use migrator_core::domain::Workspace;
use migrator_core::engine::{run_command, Command};
use migrator_core::gate::{
    evaluate_gate, max_plan_wall_ms_from_env, read_snapshot_json, resolve_changed_paths,
    write_snapshot_file, GateInput, PlanSnapshot,
};
use migrator_core::scan;

#[tokio::test]
async fn prod_gate_incremental_plan() {
    if !common::integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = common::config();
    let baseline_path = gate::prod_gate_baseline_path();

    let out = run_command(Command::Plan, cfg)
        .await
        .expect("plan for prod gate");
    let plan = out.plan.expect("migration plan");
    let current = PlanSnapshot::from_plan(&plan);

    if gate::gate_update_baseline() {
        write_snapshot_file(&baseline_path, &current).expect("write baseline");
        eprintln!("updated baseline at {}", baseline_path.display());
        return;
    }

    let baseline_data =
        std::fs::read_to_string(&baseline_path).unwrap_or_else(|e| panic!("read baseline: {e}"));
    let baseline = read_snapshot_json(&baseline_data).expect("parse baseline");

    let mut ws = Workspace::default();
    scan::populate(&mut ws, &cfg.sql_root, cfg.skip_git())
        .await
        .expect("scan");
    let paths = resolve_changed_paths(&cfg.sql_root);
    let mut delta = migrator_core::gate::keys_for_changed_paths(&ws, &paths.paths);
    delta = migrator_core::gate::expand_delta_closure(&ws, delta);
    eprintln!(
        "delta source: {} (full_inspect={})",
        paths.source, paths.full_inspect
    );

    let result = evaluate_gate(GateInput {
        blocked: plan.blocked,
        timings: out.timings.clone(),
        max_plan_wall_ms: max_plan_wall_ms_from_env(),
        baseline,
        current,
        delta_keys: delta,
    });

    let report = serde_json::to_string_pretty(&result).expect("marshal gate result");
    eprintln!("prod gate result:\n{report}");

    if let Ok(path) = std::env::var("RMIG_GATE_REPORT") {
        if !path.is_empty() {
            let p = std::path::Path::new(&path);
            if let Some(parent) = p.parent() {
                let _ = std::fs::create_dir_all(parent);
            }
            std::fs::write(&path, format!("{report}\n")).expect("write RMIG_GATE_REPORT");
            eprintln!("wrote gate report to {path}");
        }
    }

    if !result.passed {
        for msg in &result.messages {
            eprintln!("no-go: {msg}");
        }
        panic!("prod gate: NO-GO");
    }
    eprintln!("prod gate: GO");
}
