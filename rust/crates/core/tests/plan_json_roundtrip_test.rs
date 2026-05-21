use migrator_core::domain::Action;
use migrator_core::export::{read_plan_json, MigrationPlan, PlanJsonFromObjects, PlannedObject};

#[test]
fn migration_plan_checksum_base64_roundtrip() {
    let cs = [0xab; 32];
    let mut plan = MigrationPlan::default();
    plan.planned_at = "2026-01-01T00:00:00Z".into();
    plan.objects = vec![PlannedObject {
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
        }];
    let json = serde_json::to_string(&PlanJsonFromObjects(&plan)).unwrap();
    assert!(json.contains("q6s="), "checksum must be base64 on wire");
    let back = read_plan_json(&json).expect("deserialize plan JSON");
    assert_eq!(back.objects[0].checksum, cs);
}
