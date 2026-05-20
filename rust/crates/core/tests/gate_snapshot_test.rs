use std::collections::HashSet;

use migrator_core::domain::Action;
use migrator_core::export::{MigrationPlan, PlannedObject};
use migrator_core::gate::{compare_snapshots, CompareOptions, PlanSnapshot};

#[test]
fn snapshot_matches_go_baseline_shape() {
    let mut plan = MigrationPlan::default();
    plan.objects.push(PlannedObject {
        normalized_key: "smoke/tables/smoke_table".into(),
        object_path: "dactests/smoke/tables/smoke_table.sql".into(),
        schema_name: "smoke".into(),
        kind: "tables".into(),
        object_name: "smoke_table".into(),
        database_name: Default::default(),
        parent_name: Default::default(),
        planned_action: Action::CreateObject,
        exists: false,
        checksum: [0x75; 32],
        git: None,
        transition_paths: Vec::new(),
    });
    let snap = PlanSnapshot::from_plan(&plan);
    assert_eq!(snap.version, "1");
    assert_eq!(
        snap.objects["smoke/tables/smoke_table"].planned_action,
        "create_object"
    );
}

#[test]
fn compare_strict_outside_delta() {
    let mut baseline = PlanSnapshot::from_plan(&MigrationPlan::default());
    let mut current = PlanSnapshot::from_plan(&MigrationPlan::default());
    baseline.objects.insert(
        "a".into(),
        migrator_core::gate::SnapshotObject {
            object_path: String::new(),
            planned_action: "skip_unchanged".into(),
            checksum_hex: String::new(),
            exists: true,
        },
    );
    baseline.objects.insert(
        "b".into(),
        migrator_core::gate::SnapshotObject {
            object_path: String::new(),
            planned_action: "skip_unchanged".into(),
            checksum_hex: String::new(),
            exists: true,
        },
    );
    current.objects.insert(
        "a".into(),
        migrator_core::gate::SnapshotObject {
            object_path: String::new(),
            planned_action: "create_object".into(),
            checksum_hex: String::new(),
            exists: false,
        },
    );
    current.objects.insert(
        "b".into(),
        migrator_core::gate::SnapshotObject {
            object_path: String::new(),
            planned_action: "skip_unchanged".into(),
            checksum_hex: String::new(),
            exists: true,
        },
    );
    let mut delta = HashSet::new();
    delta.insert("a".into());
    let ok = compare_snapshots(
        &baseline,
        &current,
        &CompareOptions {
            delta_keys: delta.clone(),
            strict_unexpected: true,
        },
    );
    assert!(ok.go);

    current.objects.get_mut("a").unwrap().planned_action = "skip_unchanged".into();
    current.objects.get_mut("b").unwrap().planned_action = "create_object".into();
    let bad = compare_snapshots(
        &baseline,
        &current,
        &CompareOptions {
            delta_keys: delta,
            strict_unexpected: true,
        },
    );
    assert!(!bad.go);
}
