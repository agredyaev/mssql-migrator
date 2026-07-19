use migrator_core::domain::{Action, ObjectEntry, ObjectKey, Workspace};
use migrator_core::plan::{resolve_plan_scenario, PlanScenario, ScenarioInput};

#[test]
fn skip_when_checksum_matches() {
    let key = ObjectKey::new("s", "views", "v");
    let ws = Workspace::default();
    let (_, obj) = ObjectEntry::with_staging_key(key.clone(), 0, [1; 32], false, 0);
    let s = resolve_plan_scenario(ScenarioInput {
        exists: true,
        prior: Some([1; 32]),
        checksum: [1; 32],
        kind_code: 6,
        obj: &obj,
        ws: &ws,
        prior_digests: &[],
        child_row_id: 1,
        has_transition_paths: false,
        live_definition_drift: false,
    });
    assert_eq!(s, PlanScenario::SkipUnchanged);
}

#[test]
fn changed_nonmodule_without_update_path_is_blocked() {
    let scenario = resolve("indexes", 5, [1; 32], [2; 32], false);
    assert_eq!(scenario, PlanScenario::StructuralChangeBlocked);
    assert_eq!(scenario.action(), Action::Fail);
    assert_eq!(scenario.blocked_delta(), 1);
}

#[test]
fn live_nonmodule_drift_is_blocked_even_when_repository_checksum_matches() {
    let scenario = resolve("tables", 3, [1; 32], [1; 32], true);
    assert_eq!(scenario, PlanScenario::LiveStructuralDriftBlocked);
    assert_eq!(scenario.action(), Action::Fail);
    assert_eq!(scenario.blocked_delta(), 1);
}

#[test]
fn live_module_drift_keeps_safe_create_or_alter_path() {
    assert_eq!(
        resolve("views", 6, [1; 32], [1; 32], true),
        PlanScenario::ModuleUpdate
    );
}

fn resolve(
    kind: &str,
    kind_code: u8,
    prior: [u8; 32],
    checksum: [u8; 32],
    live_definition_drift: bool,
) -> PlanScenario {
    let key = ObjectKey::new("s", kind, "x");
    let ws = Workspace::default();
    let (_, obj) = ObjectEntry::with_staging_key(key, 0, checksum, false, 0);
    resolve_plan_scenario(ScenarioInput {
        exists: true,
        prior: Some(prior),
        checksum,
        kind_code,
        obj: &obj,
        ws: &ws,
        prior_digests: &[],
        child_row_id: 1,
        has_transition_paths: false,
        live_definition_drift,
    })
}
