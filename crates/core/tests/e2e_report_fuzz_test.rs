use migrator_core::gate::{
    read_e2e_apply_json, read_e2e_blocked_json, read_e2e_gate_json, read_e2e_report_json,
};
use proptest::prelude::*;

const PLAN_SCENARIOS: &[&str] = &[
    "empty_db_plan",
    "warm_db_plan",
    "skip_unchanged_plan",
    "catalog_cache_plan",
];

const APPLY_SCENARIOS: &[&str] = &["apply_smoke_result", "ddl_transition_apply"];

const GATE_SCENARIOS: &[&str] = &["prod_gate_cold"];

const BLOCKED_SCENARIOS: &[&str] = &["blocked_table_plan"];

fn baseline_json(scenario: &str) -> String {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join(format!("tests/testdata/e2e/e2e_baseline_{scenario}.json"));
    std::fs::read_to_string(&path)
        .unwrap_or_else(|e| panic!("read e2e baseline {}: {e}", path.display()))
}

#[test]
fn e2e_plan_baselines_parse_as_plan_reports() {
    for scenario in PLAN_SCENARIOS {
        let report = read_e2e_report_json(&baseline_json(scenario))
            .unwrap_or_else(|e| panic!("parse plan e2e baseline {scenario}: {e}"));
        assert_eq!(report.scenario, *scenario);
        assert!(
            !report.action_counts.is_empty(),
            "plan baseline {scenario} must include action counts"
        );
    }
}

#[test]
fn e2e_apply_baselines_parse_as_apply_reports() {
    for scenario in APPLY_SCENARIOS {
        let report = read_e2e_apply_json(&baseline_json(scenario))
            .unwrap_or_else(|e| panic!("parse apply e2e baseline {scenario}: {e}"));
        assert_eq!(report.scenario, *scenario);
        assert!(
            report.applied > 0,
            "apply baseline {scenario} must record applied objects"
        );
    }
}

#[test]
fn e2e_gate_baselines_parse_as_gate_reports() {
    for scenario in GATE_SCENARIOS {
        let report = read_e2e_gate_json(&baseline_json(scenario))
            .unwrap_or_else(|e| panic!("parse gate e2e baseline {scenario}: {e}"));
        assert_eq!(report.scenario, *scenario);
        assert!(report.gate_pass, "gate baseline {scenario} must pass");
    }
}

#[test]
fn e2e_blocked_baselines_parse_as_blocked_reports() {
    for scenario in BLOCKED_SCENARIOS {
        let report = read_e2e_blocked_json(&baseline_json(scenario))
            .unwrap_or_else(|e| panic!("parse blocked e2e baseline {scenario}: {e}"));
        assert_eq!(report.scenario, *scenario);
        assert!(
            report.blocked,
            "blocked baseline {scenario} must be blocked"
        );
        assert!(
            !report.scaffold_paths.is_empty(),
            "blocked baseline {scenario} must include scaffold paths"
        );
    }
}

#[test]
fn e2e_report_readers_reject_wrong_json_shapes() {
    for input in ["[]", r#"{"scenario":42}"#] {
        assert!(
            read_e2e_report_json(input).is_err(),
            "plan reader accepted {input}"
        );
        assert!(
            read_e2e_apply_json(input).is_err(),
            "apply reader accepted {input}"
        );
        assert!(
            read_e2e_gate_json(input).is_err(),
            "gate reader accepted {input}"
        );
        assert!(
            read_e2e_blocked_json(input).is_err(),
            "blocked reader accepted {input}"
        );
    }

    assert!(
        read_e2e_report_json(r#"{"scenario":"x","timings":[]}"#).is_err(),
        "plan reader accepted invalid timings"
    );
    assert!(
        read_e2e_report_json(r#"{"scenario":"x","snapshot":[]}"#).is_err(),
        "plan reader accepted invalid snapshot"
    );
    assert!(
        read_e2e_apply_json(r#"{"scenario":"x","timings":[]}"#).is_err(),
        "apply reader accepted invalid timings"
    );
    assert!(
        read_e2e_gate_json(r#"{"scenario":"x","snapshot":[]}"#).is_err(),
        "gate reader accepted invalid snapshot"
    );
    assert!(
        read_e2e_blocked_json(r#"{"scenario":"x","timings":[]}"#).is_err(),
        "blocked reader accepted invalid timings"
    );
}

proptest! {
    #[test]
    fn e2e_report_readers_never_panic_on_fuzz_input(input in "\\PC{0,4096}") {
        let _ = read_e2e_report_json(&input);
        let _ = read_e2e_apply_json(&input);
        let _ = read_e2e_gate_json(&input);
        let _ = read_e2e_blocked_json(&input);
    }
}
