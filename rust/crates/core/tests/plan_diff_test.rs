use std::collections::HashMap;

use migrator_core::db::state::{catalog_object, CatalogState, ChecksumMap};
use migrator_core::domain::{ObjectEntry, ObjectKey, ScriptKey, Workspace};
use migrator_core::plan::compute_diff;
use migrator_core::scan::transition;

#[test]
fn table_changed_without_transitions_is_blocked() {
    let mut ws = Workspace::default();
    let key = ObjectKey::new("r", "tables", "t1");
    ws.adopt_dense_entries(vec![ObjectEntry {
        key: key.clone(),
        script: ScriptKey::from_path("r/tables/t1.sql"),
        history: None,
        db: migrator_core::domain::DbFacts {
            exists: true,
            parent: None,
        },
        plan: None,
        checksum: [1; 32],
        schema: "r".into(),
        kind: "tables".into(),
        name: "t1".into(),
        database_name: "db".into(),
        parent_name: Default::default(),
        parent_key: None,
    }]);
    let mut checksums: ChecksumMap = HashMap::new();
    checksums.insert(key.clone(), [2; 32]);
    let mut catalog = CatalogState::default();
    catalog.objects.insert(
        key.clone(),
        catalog_object("r", "tables", "t1", None),
    );
    let (plan, _) = compute_diff(&mut ws, &catalog, &checksums).unwrap();
    assert!(plan.blocked);
    assert_eq!(plan.summary.blocked_count, 1);
}

#[test]
fn transition_filename_parsed() {
    let rel = "db/r/tables/_migrations/snap/001_deadbeef_add.sql";
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("001_deadbeef_add.sql");
    std::fs::write(&path, b"ALTER TABLE t ADD x int").unwrap();
    let mut ws = Workspace::default();
    transition::ingest(&mut ws, rel, &path).unwrap();
    assert_eq!(ws.transitions_by_table.len(), 1);
}
