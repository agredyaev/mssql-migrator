//! Pipeline-level scenario e2e: behavior + phase timings vs committed baseline JSON.

mod common;

#[path = "common/pipeline.rs"]
mod pipeline;

#[path = "common/migrate.rs"]
mod migrate;

#[path = "common/e2e_verify.rs"]
mod e2e_verify;

#[path = "common/gate_e2e.rs"]
mod gate_e2e;

#[path = "common/catalog.rs"]
mod catalog;

#[path = "common/blocked.rs"]
mod blocked;

#[path = "common/gate_paths.rs"]
mod gate;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/db_reset_skip.rs"]
mod db_reset_skip;

use std::path::{Path, PathBuf};

use migrator_core::gate::{
    build_e2e_report, compare_e2e_apply_reports, compare_e2e_blocked_reports,
    compare_e2e_gate_reports, compare_e2e_reports, read_e2e_apply_json, read_e2e_blocked_json,
    read_e2e_gate_json, read_e2e_report_json, write_e2e_apply_file, write_e2e_blocked_file,
    write_e2e_gate_file, write_e2e_report_file, E2EApplyReport,
};

fn is_plan_scenario(scenario: &str) -> bool {
    matches!(
        scenario,
        "empty_db_plan" | "warm_db_plan" | "skip_unchanged_plan" | "catalog_cache_plan"
    )
}

fn default_baseline_path(scenario: &str) -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join(format!("tests/testdata/e2e/e2e_baseline_{scenario}.json"))
}

fn baseline_report_path(scenario: &str) -> PathBuf {
    if let Ok(p) = std::env::var("RMIG_E2E_BASELINE_REPORT") {
        if !p.is_empty() {
            return PathBuf::from(p);
        }
    }
    default_baseline_path(scenario)
}

#[tokio::test]
async fn apply_smoke_setup() {
    if !common::integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = common::direct_config();
    migrate::run_apply_smoke(cfg)
        .await
        .expect("apply smoke setup for warm scenarios");
}

#[tokio::test]
async fn scenario_matches_baseline() {
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
    let baseline_path = baseline_report_path(&scenario);
    let baseline_data = std::fs::read_to_string(&baseline_path)
        .unwrap_or_else(|e| panic!("read baseline report {}: {e}", baseline_path.display()));

    if is_plan_scenario(&scenario) {
        run_plan_scenario(&scenario, &baseline_data).await;
        return;
    }

    match scenario.as_str() {
        "apply_smoke_result" => run_apply_scenario(&baseline_data).await,
        "ddl_transition_apply" => run_ddl_transition_scenario(&baseline_data).await,
        "prod_gate_cold" => run_gate_scenario(&baseline_data).await,
        "blocked_table_plan" => run_blocked_scenario(&baseline_data).await,
        other => panic!("unknown RMIG_E2E_SCENARIO: {other}"),
    }
}

async fn run_plan_scenario(scenario: &str, baseline_data: &str) {
    let baseline_rep = read_e2e_report_json(baseline_data).expect("parse baseline e2e report");
    let cfg = common::direct_config();
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(cfg)
            .await
            .expect("reset db for plan scenario");
    }
    if scenario == "warm_db_plan" {
        pipeline::run_plan_pipeline(cfg)
            .await
            .expect("warm SQL Server fingerprint query plan");
    }
    let (plan, timings, io) = pipeline::run_plan_pipeline(cfg)
        .await
        .expect("plan pipeline");
    let rust_rep = build_e2e_report(scenario, &plan, &timings, &io);

    write_rust_report(&rust_rep, write_e2e_report_file);

    let diffs = compare_e2e_reports(&baseline_rep, &rust_rep);
    assert_no_diffs(scenario, &diffs);
    eprintln!(
        "e2e scenario {:?} OK: actions={:?} plan_wall baseline={}ms actual={}ms",
        scenario,
        rust_rep.action_counts,
        baseline_rep.timings.plan_wall_ms,
        rust_rep.timings.plan_wall_ms,
    );
}

async fn run_apply_scenario(baseline_data: &str) {
    let baseline_rep = read_e2e_apply_json(baseline_data).expect("parse baseline apply report");
    let cfg = common::direct_config();
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(cfg)
            .await
            .expect("reset db for apply_smoke");
    }
    let out = migrate::run_apply_smoke(cfg).await.expect("apply smoke");
    migrate::verify_cold_apply_report(cfg, &out)
        .await
        .expect("cold apply DB invariants");
    let rust_rep = E2EApplyReport {
        scenario: "apply_smoke_result".into(),
        setup_steps: vec!["plan_pipeline".into(), "apply_execute".into()],
        applied: out.applied,
        failed: out.failed,
        skipped: out.skipped,
        errors: Vec::new(),
        audit_object_rows: out.audit_object_rows,
        audit_migration_rows: out.audit_migration_rows,
        catalog_meta_rows: out.catalog_meta_rows,
        catalog_cache_rows: out.catalog_cache_rows,
        timings: Default::default(),
    };
    write_rust_report(&rust_rep, write_e2e_apply_file);
    let diffs = compare_e2e_apply_reports(&baseline_rep, &rust_rep);
    assert_no_diffs("apply_smoke_result", &diffs);
    eprintln!(
        "e2e apply_smoke_result OK: applied={} object_rows={} migration_rows={} catalog_meta={} catalog_cache={}",
        rust_rep.applied,
        rust_rep.audit_object_rows,
        rust_rep.audit_migration_rows,
        rust_rep.catalog_meta_rows,
        rust_rep.catalog_cache_rows
    );
}

async fn run_ddl_transition_scenario(baseline_data: &str) {
    let baseline_rep = read_e2e_apply_json(baseline_data).expect("parse baseline apply report");
    let mut cfg = common::direct_config().clone();
    cfg.set_skip_git(false);
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(&cfg)
            .await
            .expect("reset db for ddl_transition_apply");
    }
    let rust_rep = blocked::run_ddl_transition_apply(&cfg)
        .await
        .expect("ddl transition apply (full workflow invariants inside)");
    write_rust_report(&rust_rep, write_e2e_apply_file);
    let diffs = compare_e2e_apply_reports(&baseline_rep, &rust_rep);
    assert_no_diffs("ddl_transition_apply", &diffs);
    eprintln!(
        "e2e ddl_transition_apply OK: object_rows={} migration_rows={} catalog_meta={} catalog_cache={} total={}ms",
        rust_rep.audit_object_rows,
        rust_rep.audit_migration_rows,
        rust_rep.catalog_meta_rows,
        rust_rep.catalog_cache_rows,
        rust_rep.timings.total_ms,
    );
}

async fn run_gate_scenario(baseline_data: &str) {
    let baseline_rep = read_e2e_gate_json(baseline_data).expect("parse baseline gate report");
    let cfg = common::direct_config();
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(cfg)
            .await
            .expect("reset db for prod_gate_cold");
    }
    let gate_baseline = gate::prod_gate_baseline_path();
    let (plan, timings, _) = pipeline::run_plan_pipeline(cfg)
        .await
        .expect("plan pipeline for gate");
    let rust_rep = gate_e2e::build_gate_report(cfg, &plan, timings, &gate_baseline)
        .await
        .expect("prod gate cold");
    write_rust_report(&rust_rep, write_e2e_gate_file);
    let diffs = compare_e2e_gate_reports(&baseline_rep, &rust_rep);
    assert_no_diffs("prod_gate_cold", &diffs);
    eprintln!("e2e prod_gate_cold OK: gate_pass={}", rust_rep.gate_pass);
}

async fn run_blocked_scenario(baseline_data: &str) {
    let baseline_rep = read_e2e_blocked_json(baseline_data).expect("parse baseline blocked report");
    let cfg = common::direct_config();
    if !db_reset_skip::skip_db_reset() {
        db_reset::reset_test_database(cfg)
            .await
            .expect("reset db for blocked_table_plan");
    }
    let mut blocked_cfg = cfg.clone();
    blocked_cfg.set_skip_git(false);
    let rust_rep = blocked::run_blocked_table_plan(&blocked_cfg)
        .await
        .expect("blocked table plan");
    write_rust_report(&rust_rep, write_e2e_blocked_file);
    let diffs = compare_e2e_blocked_reports(&baseline_rep, &rust_rep);
    assert_no_diffs("blocked_table_plan", &diffs);
    eprintln!(
        "e2e blocked_table_plan OK: exit={} scaffolds={} setup={}ms plan_par={}ms migrate_par={}ms total={}ms path={}",
        rust_rep.exit_code,
        rust_rep.scaffold_paths.len(),
        rust_rep.timings.setup_apply_ms,
        rust_rep.timings.plan_parallel_wall_ms,
        rust_rep.timings.migrate_parallel_wall_ms,
        rust_rep.timings.total_ms,
        rust_rep.timings.plan_db_path,
    );
}

fn write_rust_report<T, F>(rep: &T, write: F)
where
    F: FnOnce(&std::path::Path, &T) -> std::io::Result<()>,
{
    if let Ok(out) = std::env::var("RMIG_E2E_REPORT") {
        if !out.is_empty() {
            write(std::path::Path::new(&out), rep).expect("write e2e report");
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
    panic!("e2e scenario {scenario:?} mismatch ({} diffs)", diffs.len());
}
