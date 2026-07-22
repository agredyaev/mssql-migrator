use migrator_core::db::state::{catalog_object, CatalogState, ChecksumMap};
use migrator_core::domain::{
    share, Action, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, Workspace,
};
use migrator_core::plan::compute_diff;
use migrator_core::scan::transition;

#[test]
fn table_changed_without_transitions_is_blocked() {
    let mut ws = Workspace::default();
    let key = ObjectKey::new("r", "tables", "t1");
    let script_id = ws.insert_script(Script {
        key: ScriptKey::from_path("r/tables/t1.sql"),
        kind: ScriptKind::Object,
        abs_path: share("r/tables/t1.sql"),
        checksum: Some([1; 32]),
        scaffold: false,
    });
    let db_id = ws.intern_database(share("db"));
    ws.adopt_dense_entries(vec![ObjectEntry::with_staging_key(
        key.clone(),
        script_id,
        [1; 32],
        true,
        db_id,
    )]);
    let mut checksums = ChecksumMap::new();
    checksums.insert_key(&key, [2; 32]);
    let mut catalog = CatalogState::default();
    catalog
        .objects
        .insert(key.clone(), catalog_object("r", "tables", "t1", None));
    let (plan, _) = compute_diff(&mut ws, &catalog, &checksums).unwrap();
    assert!(plan.blocked);
    assert_eq!(plan.summary.blocked_count, 1);
}

#[test]
fn changed_index_without_safe_update_path_is_blocked_before_apply() {
    let mut ws = Workspace::default();
    let key = ObjectKey::new("r", "indexes", "ix_t1");
    let script_id = ws.insert_script(Script {
        key: ScriptKey::from_path("r/indexes/ix_t1.sql"),
        kind: ScriptKind::Object,
        abs_path: share("r/indexes/ix_t1.sql"),
        checksum: Some([1; 32]),
        scaffold: false,
    });
    let db_id = ws.intern_database(share("db"));
    ws.adopt_dense_entries(vec![ObjectEntry::with_staging_key(
        key.clone(),
        script_id,
        [1; 32],
        true,
        db_id,
    )]);
    let mut checksums = ChecksumMap::new();
    checksums.insert_key(&key, [2; 32]);
    let mut catalog = CatalogState::default();
    catalog
        .objects
        .insert(key, catalog_object("r", "indexes", "ix_t1", Some("t1")));

    let (mut plan, _) = compute_diff(&mut ws, &catalog, &checksums).unwrap();
    plan.ensure_objects_materialized(&ws);
    assert!(plan.blocked);
    assert_eq!(plan.summary.blocked_count, 1);
    assert_eq!(plan.objects[0].planned_action, Action::Fail);
    assert!(plan.blockers[0].contains("no safe in-place update path"));
}

#[test]
fn transition_filename_parsed() {
    let rel = "db/r/tables/_migrations/snap/001_deadbeef_add.sql";
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("001_deadbeef_add.sql");
    std::fs::write(&path, b"ALTER TABLE t ADD x int").unwrap();
    let mut ws = Workspace::default();
    transition::ingest(&mut ws, rel, &path).unwrap();
    assert_eq!(ws.script_rows.len(), 1);
}
