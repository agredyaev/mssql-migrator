use std::collections::HashMap;

use migrator_core::domain::{Action, Script, ScriptKey, ScriptKind, Workspace};
use migrator_core::export::filter_applied_migrations_on_plan;
use migrator_core::export::{MigrationPlan, PlannedObject};

const T1: &str = "r/tables/_migrations/t1/001_abc_def.sql";
const T2: &str = "r/tables/_migrations/t1/002_abc_ghi.sql";

fn plan_with_two_transitions() -> MigrationPlan {
    MigrationPlan {
        objects: vec![PlannedObject {
            normalized_key: "r/tables/t1".into(),
            object_path: "r/tables/t1.sql".into(),
            schema_name: "r".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            planned_action: Action::ReprocessChanged,
            exists: true,
            checksum: [1; 32],
            transition_paths: vec![T1.into(), T2.into()],
            database_name: Default::default(),
            parent_name: Default::default(),
            git: None,
        }],
        ..Default::default()
    }
}

fn ws_with_transition(path: &str, checksum: [u8; 32]) -> Workspace {
    let mut ws = Workspace::default();
    ws.insert_script(Script {
        key: ScriptKey::from_path(path),
        kind: ScriptKind::Transition,
        abs_path: path.to_owned(),
        checksum: Some(checksum),
    });
    ws
}

#[test]
fn drops_already_applied_transition_paths() {
    let mut plan = plan_with_two_transitions();
    let ws = ws_with_transition(T1, [7; 32]);
    let mut applied = HashMap::new();
    applied.insert(T1.to_string(), hex_of([7; 32]));
    filter_applied_migrations_on_plan(&mut plan, &ws, &applied)
        .expect("matching checksum is not tampering");
    assert_eq!(plan.objects[0].transition_paths.len(), 1);
    assert!(plan.objects[0].transition_paths[0].contains("002"));
}

/// A legacy history row without a stored checksum stays trusted as applied.
#[test]
fn empty_recorded_checksum_is_trusted_edge_case() {
    let mut plan = plan_with_two_transitions();
    let ws = ws_with_transition(T1, [7; 32]);
    let mut applied = HashMap::new();
    applied.insert(T1.to_string(), String::new());
    filter_applied_migrations_on_plan(&mut plan, &ws, &applied)
        .expect("legacy rows have nothing to compare");
    assert_eq!(plan.objects[0].transition_paths.len(), 1);
}

/// An applied transition whose file was edited afterwards must fail loudly
/// instead of being silently dropped from the plan.
#[test]
fn edited_applied_transition_is_reported_as_tampered_regression() {
    let mut plan = plan_with_two_transitions();
    let ws = ws_with_transition(T1, [9; 32]);
    let mut applied = HashMap::new();
    applied.insert(T1.to_string(), hex_of([7; 32]));
    let tampered = filter_applied_migrations_on_plan(&mut plan, &ws, &applied)
        .expect_err("changed bytes under an applied path must be rejected");
    assert_eq!(tampered, vec![T1.to_string()]);
}

fn hex_of(cs: [u8; 32]) -> String {
    cs.iter().map(|b| format!("{b:02x}")).collect()
}
