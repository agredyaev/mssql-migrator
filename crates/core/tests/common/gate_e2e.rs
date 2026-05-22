//! Prod gate e2e helper (prod_gate_cold scenario).

use std::path::Path;

use migrator_core::config::Config;
use migrator_core::domain::Workspace;
use migrator_core::error::Result;
use migrator_core::export::MigrationPlan;
use migrator_core::gate::{
    evaluate_gate, max_plan_wall_ms_from_env, read_snapshot_json, E2EGateReport, GateInput,
    PlanSnapshot,
};
use migrator_core::scan;
use migrator_core::timings::PhaseTimings;

pub async fn build_gate_report(
    cfg: &Config,
    plan: &MigrationPlan,
    timings: PhaseTimings,
    baseline_path: &Path,
) -> Result<E2EGateReport> {
    let current = PlanSnapshot::from_plan(plan);

    let baseline_data =
        std::fs::read_to_string(baseline_path).map_err(migrator_core::error::Error::Io)?;
    let baseline = read_snapshot_json(&baseline_data)
        .map_err(|e| migrator_core::error::Error::Other(e.into()))?;

    let mut ws = Workspace::default();
    scan::populate(&mut ws, &cfg.sql_root, cfg.skip_git()).await?;
    let paths = migrator_core::gate::resolve_changed_paths(&cfg.sql_root);
    let mut delta = migrator_core::gate::keys_for_changed_paths(&ws, &paths.paths);
    delta = migrator_core::gate::expand_delta_closure(&ws, delta);

    let result = evaluate_gate(GateInput {
        blocked: plan.blocked,
        timings,
        max_plan_wall_ms: max_plan_wall_ms_from_env(),
        baseline,
        current: current.clone(),
        delta_keys: delta,
    });

    Ok(E2EGateReport {
        scenario: "prod_gate_cold".into(),
        setup_steps: vec![
            "reset_db".into(),
            "plan_pipeline".into(),
            "gate_evaluate".into(),
        ],
        gate_pass: result.passed,
        messages: result.messages,
        snapshot: current,
    })
}
