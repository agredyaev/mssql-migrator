use migrator_core::db::state::ChecksumMap;
use migrator_core::domain::{
    share, ObjectEntry, ObjectKey, ParentRef, Script, ScriptKey, ScriptKind, Workspace,
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
        abs_path: share("db/smoke/tables/t1.sql"),
        checksum: Some([1; 32]),
        scaffold: false,
    });
    let trig_sid = ws.insert_script(Script {
        key: ScriptKey::from_path("db/smoke/triggers/tr.sql"),
        kind: ScriptKind::Object,
        abs_path: share("db/smoke/triggers/tr.sql"),
        checksum: Some([2; 32]),
        scaffold: false,
    });
    let db_id = ws.intern_database(share("db"));
    ws.adopt_dense_entries(vec![
        ObjectEntry::with_staging_key(parent.clone(), parent_sid, [1; 32], false, db_id),
        ObjectEntry::with_staging_key(trig.clone(), trig_sid, [2; 32], false, db_id),
    ]);
    ws.insert_parent_row(2, ParentRef { parent_row_id: 1 });
    ws
}

#[test]
fn delta_closure_adds_trigger_parent() {
    let ws = sample_ws();
    let mut delta = keys_for_changed_paths(&ws, &["db/smoke/triggers/tr.sql".into()]);
    delta = expand_delta_closure(&ws, delta);
    assert!(delta.contains("smoke/tables/t1"));
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
