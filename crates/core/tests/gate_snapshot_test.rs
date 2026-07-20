use std::collections::HashSet;

use migrator_core::domain::Action;
use migrator_core::export::{MigrationPlan, PlannedObject};
use migrator_core::gate::{compare_snapshots, CompareOptions, PlanSnapshot};

#[test]
fn snapshot_wire_shape() {
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
    assert_eq!(snap.version, "2");
    assert_eq!(
        snap.objects["smoke/tables/smoke_table"].planned_action,
        "create_object"
    );
}

fn snap_obj(action: &str, exists: bool) -> migrator_core::gate::SnapshotObject {
    migrator_core::gate::SnapshotObject {
        object_path: String::new(),
        planned_action: action.into(),
        checksum_hex: String::new(),
        exists,
    }
}

/// An empty delta is exact-match mode: any changed key fails the strict gate
/// instead of being treated as "everything allowed".
#[test]
fn compare_empty_delta_requires_exact_match_regression() {
    let mut baseline = PlanSnapshot::from_plan(&MigrationPlan::default());
    let mut current = PlanSnapshot::from_plan(&MigrationPlan::default());
    baseline
        .objects
        .insert("a".into(), snap_obj("skip_unchanged", true));
    current
        .objects
        .insert("a".into(), snap_obj("create_object", false));
    let res = compare_snapshots(
        &baseline,
        &current,
        &CompareOptions {
            delta_keys: HashSet::new(),
            strict_unexpected: true,
        },
    );
    assert!(!res.passed, "empty delta + change must fail");
    assert_eq!(res.unexpected.len(), 1, "the changed key is reported");
}

/// Snapshots with different format versions must refuse to compare.
#[test]
fn compare_rejects_version_mismatch_regression() {
    let mut baseline = PlanSnapshot::from_plan(&MigrationPlan::default());
    baseline.version = "999".into();
    let current = PlanSnapshot::from_plan(&MigrationPlan::default());
    let res = compare_snapshots(&baseline, &current, &CompareOptions::default());
    assert!(!res.passed);
    assert!(
        res.messages[0].contains("version mismatch"),
        "message: {:?}",
        res.messages
    );
}

/// Same-named objects in two catalog databases keep separate snapshot entries.
#[test]
fn snapshot_keys_are_database_qualified_regression() {
    let mut plan = MigrationPlan::default();
    for (db, action) in [
        ("db_a", Action::CreateObject),
        ("db_b", Action::SkipUnchanged),
    ] {
        plan.objects.push(PlannedObject {
            normalized_key: "s/views/v".into(),
            object_path: format!("{db}/s/views/v.sql").into(),
            schema_name: "s".into(),
            kind: "views".into(),
            object_name: "v".into(),
            database_name: db.into(),
            parent_name: Default::default(),
            planned_action: action,
            exists: false,
            checksum: [1; 32],
            git: None,
            transition_paths: Vec::new(),
        });
    }
    let snap = PlanSnapshot::from_plan(&plan);
    assert_eq!(
        snap.objects.len(),
        2,
        "no same-key collapse across databases"
    );
    assert!(snap.objects.contains_key("db_a/s/views/v"));
    assert!(snap.objects.contains_key("db_b/s/views/v"));
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
    assert!(ok.passed);

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
    assert!(!bad.passed);
}
