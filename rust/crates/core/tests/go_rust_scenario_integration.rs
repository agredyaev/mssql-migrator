//! Pipeline-level scenario e2e: behavior (action counts) + phase timings vs Go reference.

mod common;

#[path = "common/pipeline.rs"]
mod pipeline;

#[path = "common/migrate.rs"]
mod migrate;

#[path = "common/gate_e2e.rs"]
mod gate_e2e;

#[path = "common/blocked.rs"]
mod blocked;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/db_reset_skip.rs"]
mod db_reset_skip;

use std::collections::HashMap;

use migrator_core::gate::{
    build_e2e_report, compare_e2e_apply_reports, compare_e2e_blocked_reports,
    compare_e2e_gate_reports, compare_e2e_reports, read_e2e_apply_json, read_e2e_blocked_json,
    read_e2e_gate_json, read_e2e_report_json, write_e2e_apply_file, write_e2e_blocked_file,
    write_e2e_gate_file, write_e2e_report_file, E2EApplyReport,
};

fn expected_actions(scenario: &str) -> HashMap<String, i32> {
    match scenario {
        "empty_db_plan" => HashMap::from([("create_object".into(), 6)]),
        "warm_db_plan" => HashMap::from([("adopt_existing".into(), 6)]),
        "skip_unchanged_plan" => HashMap::from([("adopt_existing".into(), 6)]),
        "catalog_cache_plan" => HashMap::from([("adopt_existing".into(), 6)]),
        _ => HashMap::new(),
    }
}

fn is_plan_scenario(scenario: &str) -> bool {
    matches!(
        scenario,
        "empty_db_plan" | "warm_db_plan" | "skip_unchanged_plan" | "catalog_cache_plan"
    )
}

#[tokio::test]
async fn go_rust_scenario_matches_go_reference() {
    if !common::integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let scenario = match std::env::var("RMIG_E2E_SCENARIO") {
        Ok(s) if !s.is_empty() => s,
        _ => {
            eprintln!("skip: set RMIG_E2E_SCENARIO");
            return;
        }
    };
    let go_path = match std::env::var("RMIG_E2E_GO_REPORT") {
        Ok(p) if !p.is_empty() => std::path::PathBuf::from(p),
        _ => {
            eprintln!("skip: set RMIG_E2E_GO_REPORT");
            return;
        }
    };

    let go_data =
        std::fs::read_to_string(&go_path).unwrap_or_else(|e| panic!("read go report: {e}"));

    if is_plan_scenario(&scenario) {
        run_plan_scenario(&scenario, &go_data).await;
        return;
    }

    match scenario.as_str() {
        "apply_smoke_result" => run_apply_scenario(&go_data).await,
        "prod_gate_cold" => run_gate_scenario(&go_data).await,
        "blocked_table_plan" => run_blocked_scenario(&go_data).await,
        other => panic!("unknown RMIG_E2E_SCENARIO: {other}"),
    }
}

async fn run_plan_scenario(scenario: &str, go_data: &str) {
    let go_rep = read_e2e_report_json(go_data).expect("parse go e2e report");
    let cfg = common::config();
    let (plan, timings, io) = pipeline::run_plan_pipeline(cfg)
        .await
        .expect("rust plan pipeline");
    let rust_rep = build_e2e_report(scenario, &plan, &timings, &io);

    write_rust_report(&rust_rep, write_e2e_report_file);

    let expected = expected_actions(scenario);
    if rust_rep.action_counts != expected && !expected.is_empty() {
        panic!(
            "rust scenario {:?} action_counts {:?} != expected {:?} (go={:?})",
            scenario, rust_rep.action_counts, expected, go_rep.action_counts
        );
    }

    let diffs = compare_e2e_reports(&go_rep, &rust_rep);
    assert_no_diffs(scenario, &diffs);
    eprintln!(
        "Go↔Rust scenario {:?} OK: actions={:?} plan_wall go={}ms rust={}ms",
        scenario,
        rust_rep.action_counts,
        go_rep.timings.plan_wall_ms,
        rust_rep.timings.plan_wall_ms,
    );
}

async fn run_apply_scenario(go_data: &str) {
    let go_rep = read_e2e_apply_json(go_data).expect("parse go apply report");
    let cfg = common::config();
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(cfg)
            .await
            .expect("reset db for apply_smoke");
    }
    let out = migrate::run_apply_smoke(cfg).await.expect("apply smoke");
    let rust_rep = E2EApplyReport {
        scenario: "apply_smoke_result".into(),
        setup_steps: vec!["plan_pipeline".into(), "apply_execute".into()],
        applied: out.applied,
        failed: out.failed,
        skipped: out.skipped,
        errors: Vec::new(),
        audit_object_rows: out.audit_object_rows,
    };
    write_rust_report(&rust_rep, write_e2e_apply_file);
    let diffs = compare_e2e_apply_reports(&go_rep, &rust_rep);
    assert_no_diffs("apply_smoke_result", &diffs);
    eprintln!(
        "Go↔Rust apply_smoke_result OK: applied={} audit_rows={}",
        rust_rep.applied, rust_rep.audit_object_rows
    );
}

async fn run_gate_scenario(go_data: &str) {
    let go_rep = read_e2e_gate_json(go_data).expect("parse go gate report");
    let cfg = common::config();
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(cfg)
            .await
            .expect("reset db for prod_gate_cold");
    }
    let baseline =
        common::repo_root().join("internal/app/testdata/prod_gate/plan_baseline_empty_db.json");
    let (plan, timings, _) = pipeline::run_plan_pipeline(cfg)
        .await
        .expect("rust plan pipeline for gate");
    let rust_rep = gate_e2e::build_gate_report(cfg, &plan, timings, &baseline)
        .await
        .expect("prod gate cold");
    write_rust_report(&rust_rep, write_e2e_gate_file);
    let diffs = compare_e2e_gate_reports(&go_rep, &rust_rep);
    assert_no_diffs("prod_gate_cold", &diffs);
    eprintln!("Go↔Rust prod_gate_cold OK: gate_go={}", rust_rep.gate_go);
}

async fn run_blocked_scenario(go_data: &str) {
    let go_rep = read_e2e_blocked_json(go_data).expect("parse go blocked report");
    let cfg = common::config();
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(cfg)
            .await
            .expect("reset db for blocked_table_plan");
    }
    let mut blocked_cfg = cfg.clone();
    blocked_cfg.skip_git = false;
    let rust_rep = blocked::run_blocked_table_plan(&blocked_cfg)
        .await
        .expect("blocked table plan");
    write_rust_report(&rust_rep, write_e2e_blocked_file);
    let diffs = compare_e2e_blocked_reports(&go_rep, &rust_rep);
    assert_no_diffs("blocked_table_plan", &diffs);
    eprintln!(
        "Go↔Rust blocked_table_plan OK: exit={} scaffolds={}",
        rust_rep.exit_code,
        rust_rep.scaffold_paths.len()
    );
}

fn write_rust_report<T, F>(rep: &T, write: F)
where
    F: FnOnce(&std::path::Path, &T) -> std::io::Result<()>,
{
    if let Ok(out) = std::env::var("RMIG_E2E_RUST_REPORT") {
        if !out.is_empty() {
            write(std::path::Path::new(&out), rep).expect("write rust report");
        }
    }
}

fn assert_no_diffs(scenario: &str, diffs: &[String]) {
    if diffs.is_empty() {
        return;
    }
    for d in diffs {
        eprintln!("scenario parity: {d}");
    }
    panic!(
        "Go↔Rust scenario {scenario:?} mismatch ({} diffs)",
        diffs.len()
    );
}
