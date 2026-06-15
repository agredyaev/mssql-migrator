use migrator_core::domain::Action;
use migrator_core::export::{
    read_plan_json, write_plan_json, MigrationPlan, PlanJsonFromObjects, PlanRow, PlanSummary,
    PlannedObject,
};
use proptest::prelude::*;

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

proptest! {
    #[test]
    fn read_plan_json_never_panics_on_fuzz_input(input in "\\PC{0,4096}") {
        let _ = read_plan_json(&input);
    }

    #[test]
    fn generated_plan_json_roundtrips(
        checksum in any::<[u8; 32]>(),
        suffix in "[a-z0-9_]{1,32}",
        exists in any::<bool>(),
    ) {
        let normalized_key = format!("smoke/tables/{suffix}");
        let object_path = format!("smoke/tables/{suffix}.sql");
        let plan = MigrationPlan {
            command: "plan".into(),
            planned_at: "2026-01-01T00:00:00Z".into(),
            objects: vec![PlannedObject {
                normalized_key: normalized_key.clone().into(),
                object_path: object_path.clone().into(),
                schema_name: "smoke".into(),
                kind: "tables".into(),
                object_name: suffix.clone().into(),
                database_name: "dactests".into(),
                parent_name: Default::default(),
                planned_action: Action::CreateObject,
                exists,
                checksum,
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

        let json = serde_json::to_string(&PlanJsonFromObjects(&plan)).unwrap();
        let back = read_plan_json(&json).expect("generated plan JSON must deserialize");

        prop_assert_eq!(back.objects.len(), 1);
        prop_assert_eq!(back.objects[0].normalized_key.as_ref(), normalized_key);
        prop_assert_eq!(back.objects[0].object_path.as_ref(), object_path);
        prop_assert_eq!(back.objects[0].checksum, checksum);
        prop_assert_eq!(back.objects[0].exists, exists);
    }
}

#[test]
fn read_plan_json_rejects_invalid_checksum_length() {
    let json = r#"{
        "command": "plan",
        "plannedAt": "2026-01-01T00:00:00Z",
        "objects": [{
            "normalizedKey": "smoke/tables/t1",
            "objectPath": "smoke/tables/t1.sql",
            "schemaName": "smoke",
            "kind": "tables",
            "objectName": "t1",
            "checksum": "AQID",
            "plannedAction": "create_object",
            "exists": false
        }]
    }"#;

    let err = read_plan_json(json).expect_err("short checksum must be rejected");
    assert!(
        err.to_string().contains("checksum must be 32 bytes"),
        "unexpected error: {err}"
    );
}
