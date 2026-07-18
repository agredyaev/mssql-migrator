use migrator_core::db::state::CatalogState;
use migrator_core::domain::{ObjectEntry, ObjectKey, Workspace};
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
        catalog: &CatalogState::default(),
        prior_digests: &[],
        child_row_id: 1,
        has_transition_paths: false,
        live_definition_drift: false,
    });
    assert_eq!(s, PlanScenario::SkipUnchanged);
}
