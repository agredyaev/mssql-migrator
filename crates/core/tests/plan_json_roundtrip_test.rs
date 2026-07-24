use migrator_core::domain::Action;
use migrator_core::export::{
    write_plan_json, MigrationPlan, PlanJsonFromObjects, PlanSummary, PlannedObject, PlannedSchema,
};
use proptest::prelude::*;

/// Re-ingest of `write_plan_json` output. Production never reads plans back, so
/// this mirror lives with the round-trip test rather than the shipped crate.
#[derive(serde::Deserialize)]
struct MigrationPlanWire {
    #[serde(default)]
    command: String,
    #[serde(rename = "plannedAt", default)]
    planned_at: String,
    #[serde(default)]
    blocked: bool,
    #[serde(default)]
    blockers: Vec<String>,
    #[serde(default)]
    schemas: Vec<PlannedSchema>,
    objects: Vec<PlannedObject>,
    #[serde(default)]
    summary: PlanSummary,
}

fn read_plan_json(s: &str) -> Result<MigrationPlan, String> {
    let wire: MigrationPlanWire = serde_json::from_str(s).map_err(|e| e.to_string())?;
    Ok(MigrationPlan {
        command: wire.command,
        planned_at: wire.planned_at,
        blockers: wire.blockers,
        schemas: wire.schemas,
        objects: wire.objects,
        summary: wire.summary,
        blocked: wire.blocked,
    })
}

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
    assert_eq!(back.objects[0].database_name, "dactests");
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
                normalized_key: normalized_key.clone(),
                object_path: object_path.clone(),
                schema_name: "smoke".into(),
                kind: "tables".into(),
                object_name: suffix.clone(),
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
        prop_assert_eq!(&back.objects[0].normalized_key, &normalized_key);
        prop_assert_eq!(&back.objects[0].object_path, &object_path);
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
