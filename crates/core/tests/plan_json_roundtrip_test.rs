use migrator_core::domain::Action;
use migrator_core::export::{
    read_plan_json, write_plan_json, MigrationPlan, PlanJsonFromObjects, PlanRow, PlanSummary,
    PlannedObject,
};

#[test]
fn migration_plan_checksum_base64_roundtrip() {
    let cs = [0xab; 32];
    let plan = MigrationPlan {
        planned_at: "2026-01-01T00:00:00Z".into(),
        objects: vec![PlannedObject {
            normalized_key: "smoke/tables/t1".into(),
            object_path: "smoke/tables/t1.sql".into(),
            schema_name: "smoke".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            database_name: Default::default(),
            parent_name: Default::default(),
            planned_action: Action::CreateObject,
            exists: false,
            checksum: cs,
            git: None,
            transition_paths: Vec::new(),
        }],
        ..Default::default()
    };
    let json = serde_json::to_string(&PlanJsonFromObjects(&plan)).unwrap();
    assert!(json.contains("q6s="), "checksum must be base64 on wire");
    let back = read_plan_json(&json).expect("deserialize plan JSON");
    assert_eq!(back.objects[0].checksum, cs);
}

#[test]
fn write_plan_json_objects_only_succeeds_without_workspace() {
    let plan = MigrationPlan {
        command: "plan".into(),
        planned_at: "2026-01-01T00:00:00Z".into(),
        objects: vec![PlannedObject {
            normalized_key: "dactests/smoke/tables/t1".into(),
            object_path: "dactests/smoke/tables/t1.sql".into(),
            schema_name: "smoke".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            database_name: "dactests".into(),
            parent_name: Default::default(),
            planned_action: Action::CreateObject,
            exists: false,
            checksum: [1; 32],
            git: None,
            transition_paths: Vec::new(),
        }],
        summary: PlanSummary {
            object_count: 1,
            create_count: 1,
            ..Default::default()
        },
        ..Default::default()
    };
    let mut buf = Vec::new();
    write_plan_json(&plan, None, &mut buf).unwrap();
    let back = read_plan_json(std::str::from_utf8(&buf).unwrap()).unwrap();
    assert_eq!(back.objects.len(), 1);
    assert_eq!(back.objects[0].database_name.as_ref(), "dactests");
}

#[test]
fn write_plan_json_materialized_rows_and_objects_happy_path() {
    let mut row = PlanRow::default();
    row.set_planned_action(Action::CreateObject);
    row.set_exists(false);
    let plan = MigrationPlan {
        command: "plan".into(),
        rows: vec![row],
        objects: vec![PlannedObject {
            normalized_key: "dactests/smoke/tables/t1".into(),
            object_path: "dactests/smoke/tables/t1.sql".into(),
            schema_name: "smoke".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            database_name: "dactests".into(),
            parent_name: Default::default(),
            planned_action: Action::CreateObject,
            exists: false,
            checksum: [2; 32],
            git: None,
            transition_paths: Vec::new(),
        }],
        summary: PlanSummary {
            object_count: 1,
            create_count: 1,
            ..Default::default()
        },
        ..Default::default()
    };
    let mut buf = Vec::new();
    write_plan_json(&plan, None, &mut buf).unwrap();
    let back = read_plan_json(std::str::from_utf8(&buf).unwrap()).unwrap();
    assert_eq!(back.objects.len(), 1);
}

#[test]
fn write_plan_json_partial_materialization_still_requires_workspace_edge_case() {
    let mut row1 = PlanRow::default();
    row1.set_planned_action(Action::CreateObject);
    let mut row2 = PlanRow::default();
    row2.set_planned_action(Action::CreateObject);
    let plan = MigrationPlan {
        command: "plan".into(),
        rows: vec![row1, row2],
        objects: vec![PlannedObject {
            normalized_key: "dactests/smoke/tables/t1".into(),
            object_path: "dactests/smoke/tables/t1.sql".into(),
            schema_name: "smoke".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            database_name: "dactests".into(),
            parent_name: Default::default(),
            planned_action: Action::CreateObject,
            exists: false,
            checksum: [2; 32],
            git: None,
            transition_paths: Vec::new(),
        }],
        ..Default::default()
    };
    let mut buf = Vec::new();
    let err = write_plan_json(&plan, None, &mut buf).unwrap_err();
    assert!(
        err.to_string()
            .contains("workspace required for slim plan rows"),
        "unexpected error: {err}"
    );
}

#[test]
fn write_plan_json_materialized_rows_and_objects_regression() {
    let mut row = PlanRow::default();
    row.set_planned_action(Action::CreateObject);
    let plan = MigrationPlan {
        command: "plan".into(),
        rows: vec![row],
        objects: vec![PlannedObject {
            normalized_key: "containedplan/smoke/tables/t1".into(),
            object_path: "containedplan/smoke/tables/t1.sql".into(),
            schema_name: "smoke".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            database_name: "containedplan".into(),
            parent_name: Default::default(),
            planned_action: Action::CreateObject,
            exists: false,
            checksum: [9; 32],
            git: None,
            transition_paths: Vec::new(),
        }],
        ..Default::default()
    };
    let mut buf = Vec::new();
    write_plan_json(&plan, None, &mut buf).expect("BG-017 regression: no workspace needed");
}

#[test]
fn write_plan_json_slim_rows_without_workspace_or_objects_fails() {
    let mut row = PlanRow::default();
    row.set_planned_action(Action::CreateObject);
    row.set_exists(false);
    let plan = MigrationPlan {
        command: "plan".into(),
        rows: vec![row],
        summary: PlanSummary {
            object_count: 1,
            create_count: 1,
            ..Default::default()
        },
        ..Default::default()
    };
    let mut buf = Vec::new();
    let err = write_plan_json(&plan, None, &mut buf).unwrap_err();
    assert!(
        err.to_string()
            .contains("workspace required for slim plan rows"),
        "unexpected error: {err}"
    );
}
