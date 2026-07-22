use std::fs;

use migrator_core::config::Config;
use migrator_core::domain::{
    share, Action, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, Workspace,
};
use migrator_core::export::{write_reports, MigrationPlan, PlanRow, PlannedObject};

#[test]
fn writes_plan_and_report_json() {
    let dir = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.report_dir = dir.path().to_string_lossy().into();
    let plan = MigrationPlan {
        command: "plan".into(),
        blocked: true,
        objects: vec![PlannedObject {
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
        }],
        ..Default::default()
    };
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

#[test]
fn slim_row_plan_without_workspace_fails_report_write() {
    let dir = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.report_dir = dir.path().to_string_lossy().into();
    let plan = MigrationPlan {
        command: "plan".into(),
        rows: vec![PlanRow::default()],
        summary: Default::default(),
        ..Default::default()
    };
    let err = write_reports(&cfg, "plan", Some(&plan), None, 0).unwrap_err();
    assert!(
        err.to_string()
            .contains("workspace required for slim plan rows"),
        "unexpected error: {err}"
    );
}

#[test]
fn slim_row_plan_with_workspace_writes_plan_json() {
    let dir = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.report_dir = dir.path().to_string_lossy().into();

    let mut ws = Workspace::default();
    let script_id = ws.insert_script(Script {
        key: ScriptKey::from_path("dactests/r/tables/t1.sql"),
        kind: ScriptKind::Object,
        abs_path: share("dactests/r/tables/t1.sql"),
        checksum: None,
    });
    let db_id = ws.intern_database(share("dactests"));
    ws.adopt_dense_entries(vec![ObjectEntry::with_staging_key(
        ObjectKey::new("r", "tables", "t1"),
        script_id,
        [0; 32],
        false,
        db_id,
    )]);

    let mut row = PlanRow::default();
    row.set_planned_action(Action::CreateObject);
    row.set_exists(false);

    let plan = MigrationPlan {
        command: "plan".into(),
        rows: vec![row],
        summary: migrator_core::export::PlanSummary {
            object_count: 1,
            create_count: 1,
            ..Default::default()
        },
        ..Default::default()
    };

    write_reports(&cfg, "plan", Some(&plan), Some(&ws), 0).unwrap();
    let plan_data = fs::read_to_string(dir.path().join(".plan.json")).unwrap();
    assert!(
        plan_data.contains("\"databaseName\": \"dactests\""),
        "{plan_data}"
    );
    assert!(
        plan_data.contains("\"objectPath\": \"dactests/r/tables/t1.sql\""),
        "{plan_data}"
    );
    assert!(
        plan_data.contains("\"plannedAction\": \"create_object\""),
        "{plan_data}"
    );
}

#[test]
fn slim_row_plan_with_materialized_objects_writes_report_without_workspace() {
    let dir = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.report_dir = dir.path().to_string_lossy().into();

    let mut row = PlanRow::default();
    row.set_planned_action(Action::CreateObject);
    row.set_exists(false);

    let plan = MigrationPlan {
        command: "plan".into(),
        rows: vec![row],
        objects: vec![PlannedObject {
            normalized_key: "dactests/r/tables/t1".into(),
            object_path: "dactests/r/tables/t1.sql".into(),
            schema_name: "r".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            database_name: "dactests".into(),
            parent_name: Default::default(),
            planned_action: Action::CreateObject,
            exists: false,
            checksum: [7; 32],
            git: None,
            transition_paths: Vec::new(),
        }],
        summary: migrator_core::export::PlanSummary {
            object_count: 1,
            create_count: 1,
            ..Default::default()
        },
        ..Default::default()
    };

    write_reports(&cfg, "plan", Some(&plan), None, 0).unwrap();
    let plan_data = fs::read_to_string(dir.path().join(".plan.json")).unwrap();
    assert!(
        plan_data.contains("\"databaseName\": \"dactests\""),
        "{plan_data}"
    );
    assert!(
        plan_data.contains("\"objectPath\": \"dactests/r/tables/t1.sql\""),
        "{plan_data}"
    );
}

/// A failed run (no plan) must remove the previous run's `.plan.json`, so the
/// failure report is never diagnosed against a stale plan.
#[test]
fn failed_run_removes_stale_plan_regression() {
    let dir = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.report_dir = dir.path().to_string_lossy().into();
    let plan = MigrationPlan {
        command: "plan".into(),
        ..Default::default()
    };
    write_reports(&cfg, "plan", Some(&plan), None, 0).expect("success run");
    assert!(dir.path().join(".plan.json").exists());

    write_reports(&cfg, "migrate", None, None, 5).expect("failure run");
    assert!(
        !dir.path().join(".plan.json").exists(),
        "stale plan must be removed on a plan-less failure"
    );
    assert!(dir.path().join(".report.json").exists());
}

/// Two concurrent writers into one report dir must both succeed (unique temp
/// names; last complete writer wins).
#[test]
fn concurrent_report_writers_both_succeed_regression() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().to_string_lossy().into_owned();
    let barrier = std::sync::Arc::new(std::sync::Barrier::new(2));
    let handles: Vec<_> = (0..2)
        .map(|i| {
            let barrier = barrier.clone();
            let report_dir = path.clone();
            std::thread::spawn(move || {
                let mut cfg = Config::default();
                cfg.report_dir = report_dir;
                let plan = MigrationPlan {
                    command: format!("plan{i}"),
                    ..Default::default()
                };
                barrier.wait();
                write_reports(&cfg, "plan", Some(&plan), None, 0)
            })
        })
        .collect();
    for h in handles {
        h.join()
            .expect("thread")
            .expect("both writers must succeed");
    }
    assert!(dir.path().join(".plan.json").exists());
    assert!(dir.path().join(".report.json").exists());
}
