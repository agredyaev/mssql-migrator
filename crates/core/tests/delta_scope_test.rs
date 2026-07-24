use migrator_core::db::state::ChecksumMap;
use migrator_core::domain::{
    ObjectEntry, ObjectKey, ParentRef, Script, ScriptKey, ScriptKind, Workspace,
};
use migrator_core::gate::{expand_delta_closure, keys_for_changed_paths};
use migrator_core::plan::scope::build_inspect_scope;

fn sample_ws() -> Workspace {
    let mut ws = Workspace::default();
    let parent = ObjectKey::new("smoke", "tables", "t1");
    let trig = ObjectKey::new("smoke", "triggers", "tr");
    let parent_sid = ws.insert_script(Script {
        key: ScriptKey::from_path("db/smoke/tables/t1.sql"),
        kind: ScriptKind::Object,
        abs_path: "db/smoke/tables/t1.sql".into(),
        checksum: Some([1; 32]),
    });
    let trig_sid = ws.insert_script(Script {
        key: ScriptKey::from_path("db/smoke/triggers/tr.sql"),
        kind: ScriptKind::Object,
        abs_path: "db/smoke/triggers/tr.sql".into(),
        checksum: Some([2; 32]),
    });
    let db_id = ws.intern_database("db".into());
    ws.adopt_dense_entries(vec![
        ObjectEntry::new(parent.clone(), parent_sid, [1; 32], false, db_id),
        ObjectEntry::new(trig.clone(), trig_sid, [2; 32], false, db_id),
    ]);
    let parent_row_id = ws.key_index(&parent);
    let trigger_index = ws.key_index(&trig) as usize - 1;
    ws.object_entries[trigger_index].parent = Some(ParentRef { parent_row_id });
    ws
}

fn wide_ws() -> Workspace {
    let mut ws = Workspace::default();
    let db_id = ws.intern_database("db".into());
    let mut entries = Vec::new();
    for i in 0..10u8 {
        let name = format!("t{i}");
        let path = format!("db/smoke/tables/{name}.sql");
        let key = ObjectKey::new("smoke", "tables", &name);
        let checksum = [i + 1; 32];
        let sid = ws.insert_script(Script {
            key: ScriptKey::from_path(&path),
            kind: ScriptKind::Object,
            abs_path: path,
            checksum: Some(checksum),
        });
        entries.push(ObjectEntry::new(key, sid, checksum, false, db_id));
    }
    ws.adopt_dense_entries(entries);
    ws
}

#[test]
fn delta_closure_adds_trigger_parent() {
    let ws = sample_ws();
    let mut delta = keys_for_changed_paths(&ws, &["db/smoke/triggers/tr.sql".into()]);
    // Delta keys are database-qualified to match snapshot identity.
    assert!(delta.contains("db/smoke/triggers/tr"), "delta={delta:?}");
    delta = expand_delta_closure(&ws, delta);
    assert!(delta.contains("db/smoke/tables/t1"), "delta={delta:?}");
}

#[test]
fn scope_git_delta_marks_hot() {
    let ws = sample_ws();
    let mut checksums = ChecksumMap::new();
    checksums.insert_normalized("smoke/tables/t1", [1; 32]);
    checksums.insert_normalized("smoke/triggers/tr", [2; 32]);
    let scope = build_inspect_scope(&ws, &["db/smoke/triggers/tr.sql".into()], false, &checksums);
    assert!(scope.hot_keys.contains("smoke/triggers/tr"));
}

/// The git-delta catalog query is scoped by this JSON: if changed objects do
/// not appear here, a git-delta migrate inspects nothing and misreads every
/// existing object as absent.
#[test]
fn git_hot_scope_json_targets_changed_objects_regression() {
    let ws = sample_ws();
    let json = migrator_core::plan::git_hot_scope_json(&ws, &["db/smoke/triggers/tr.sql".into()]);
    assert!(json.contains("\"object\":\"tr\""), "scope json={json}");
    assert!(
        json.contains("\"object\":\"t1\""),
        "delta closure parent must be inspected too: {json}"
    );
}

#[test]
fn mutating_full_scope_inspects_every_object_regression() {
    let ws = wide_ws();
    let mut checksums = ChecksumMap::new();
    for i in 0..ws.object_count() {
        checksums.insert_key(ws.entry_key(i), ws.entry(i).checksum);
    }

    let scope = build_inspect_scope(&ws, &[], true, &checksums);
    assert!(scope.full_inspect);
    assert_eq!(scope.hot_keys.len(), 10, "all managed objects must be live");
    assert!(
        scope.stable_objects.is_empty(),
        "mutating plans must not synthesize stable catalog objects"
    );
}
