use migrator_core::db::state::ChecksumMap;
use migrator_core::domain::{ObjectEntry, ObjectKey, ScriptKey, Workspace};
use migrator_core::gate::{expand_delta_closure, keys_for_changed_paths};
use migrator_core::plan::scope::build_inspect_scope;

fn sample_ws() -> Workspace {
    let mut ws = Workspace::default();
    let parent = ObjectKey::new("smoke", "tables", "t1");
    let trig = ObjectKey::new("smoke", "triggers", "tr");
    ws.adopt_dense_entries(vec![
        ObjectEntry {
            key: parent.clone(),
            script: ScriptKey::from_path("db/smoke/tables/t1.sql"),
            history: None,
            db: Default::default(),
            plan: None,
            checksum: [1; 32],
            schema: "smoke".into(),
            kind: "tables".into(),
            name: "t1".into(),
            database_name: "db".into(),
            parent_name: Default::default(),
            parent_key: None,
        },
        ObjectEntry {
            key: trig,
            script: ScriptKey::from_path("db/smoke/triggers/tr.sql"),
            history: None,
            db: Default::default(),
            plan: None,
            checksum: [2; 32],
            schema: "smoke".into(),
            kind: "triggers".into(),
            name: "tr".into(),
            database_name: "db".into(),
            parent_name: "t1".into(),
            parent_key: Some(parent),
        },
    ]);
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
    let mut checksums: ChecksumMap = std::collections::HashMap::new();
    checksums.insert(ObjectKey::from_normalized("smoke/tables/t1"), [1; 32]);
    checksums.insert(ObjectKey::from_normalized("smoke/triggers/tr"), [2; 32]);
    let scope = build_inspect_scope(
        &ws,
        &["db/smoke/triggers/tr.sql".into()],
        false,
        &checksums,
    );
    assert!(scope.hot_keys.contains("smoke/triggers/tr"));
}
