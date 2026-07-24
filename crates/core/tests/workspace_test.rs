use migrator_core::domain::{ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, Workspace};

#[test]
fn for_catalog_database_remaps_db_id() {
    let mut ws = Workspace::default();
    let db_id = ws.intern_database("dactests".into());
    let script_id = ws.insert_script(Script {
        key: ScriptKey::from_path("dactests/smoke/tables/t1.sql"),
        kind: ScriptKind::Object,
        abs_path: "dactests/smoke/tables/t1.sql".into(),
        checksum: None,
    });
    ws.adopt_dense_entries(vec![ObjectEntry::new(
        ObjectKey::new("smoke", "tables", "t1"),
        script_id,
        [0; 32],
        true,
        db_id,
    )]);
    let sub = ws.for_catalog_database("dactests");
    assert_eq!(sub.object_count(), 1);
    assert_eq!(sub.database_name(sub.entry(0).db_id), "dactests");
}

#[test]
fn scan_links_non_scaffold_transition_to_table() {
    let base = tempfile::tempdir().unwrap();
    let root = base.path();
    let table_path = root.join("dactests/smoke/tables/smoke_table.sql");
    std::fs::create_dir_all(table_path.parent().unwrap()).unwrap();
    std::fs::write(
        &table_path,
        "CREATE TABLE smoke.smoke_table (\n    id INT NOT NULL,\n    added_at DATETIME2 NULL\n);",
    )
    .unwrap();
    let mig_dir = root.join("dactests/smoke/tables/_migrations/smoke_table");
    std::fs::create_dir_all(&mig_dir).unwrap();
    std::fs::write(
        mig_dir.join("001_a1b2c3d_auto_add_columns.sql"),
        "-- Auto-generated migration\nALTER TABLE [smoke].[smoke_table] ADD [added_at] DATETIME2 NULL;\n",
    )
    .unwrap();

    let mut ws = Workspace::default();
    migrator_core::scan::scan_root(&mut ws, root.to_str().unwrap()).unwrap();
    let key = ObjectKey::new("smoke", "tables", "smoke_table");
    let row = ws.key_index(&key);
    assert!(row > 0, "smoke_table row missing");
    assert!(
        ws.row_has_transition_paths(row as usize - 1),
        "transition paths flag unset"
    );

    let sub = ws.for_catalog_database("dactests");
    let sub_row = sub.key_index(&key);
    assert!(
        sub.row_has_transition_paths(sub_row as usize - 1),
        "subset lost transition paths"
    );
}
