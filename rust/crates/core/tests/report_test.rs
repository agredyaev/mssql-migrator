use std::fs;

use migrator_core::config::Config;
use migrator_core::domain::Action;
use migrator_core::export::{write_reports, MigrationPlan, PlannedObject};

#[test]
fn writes_plan_and_report_json() {
    let dir = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.report_dir = dir.path().to_string_lossy().into();
    let mut plan = MigrationPlan::default();
    plan.command = "plan".into();
    plan.blocked = true;
    plan.objects = vec![PlannedObject {
            normalized_key: "r/tables/t1".into(),
            object_path: "r/tables/t1.sql".into(),
            schema_name: "r".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            planned_action: Action::ReprocessChangedBlocked,
            exists: true,
            checksum: [0; 32],
            database_name: Default::default(),
            parent_name: Default::default(),
            git: None,
            transition_paths: Vec::new(),
        }];
    write_reports(&cfg, "plan", Some(&plan), None, 0).unwrap();
    let plan_data = fs::read_to_string(dir.path().join(".plan.json")).unwrap();
    assert!(plan_data.contains("\"blocked\": true"));
    let report_data = fs::read_to_string(dir.path().join(".report.json")).unwrap();
    let report: serde_json::Value = serde_json::from_str(&report_data).unwrap();
    assert_eq!(report["command"], "plan");
    assert_eq!(report["result"], "success");
    assert_eq!(report["exitCode"], 0);
}

#[test]
fn writes_failure_report_without_plan() {
    let dir = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.report_dir = dir.path().to_string_lossy().into();
    write_reports(&cfg, "migrate", None, None, 5).unwrap();
    assert!(fs::metadata(dir.path().join(".plan.json")).is_err());
    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join(".report.json")).unwrap())
            .unwrap();
    assert_eq!(report["result"], "failure");
    assert_eq!(report["exitCode"], 5);
}
