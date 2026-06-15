use crate::domain::Action;
use crate::engine::run::merge::merge_plan;
use crate::export::{MigrationPlan, PlanRow, PlannedObject};

fn plan_with_db(db: &str, key: &str) -> MigrationPlan {
    let mut plan = MigrationPlan::default();
    plan.summary.object_count = 1;
    plan.rows.push(PlanRow::default());
    plan.objects.push(PlannedObject {
        normalized_key: key.into(),
        object_path: key.into(),
        schema_name: "smoke".into(),
        kind: "table".into(),
        object_name: key.into(),
        database_name: db.into(),
        parent_name: "".into(),
        transition_paths: vec![],
        git: None,
        checksum: [0; 32],
        planned_action: Action::CreateObject,
        exists: false,
    });
    plan
}

#[test]
fn merge_plan_accumulates_objects_happy_path() {
    let mut merged: Option<MigrationPlan> = None;
    merge_plan(&mut merged, plan_with_db("dactests", "smoke_table"));
    merge_plan(&mut merged, plan_with_db("warehouse", "fact_table"));
    let plan = merged.expect("merged plan");
    assert_eq!(plan.objects.len(), 2);
    assert_eq!(plan.summary.object_count, 2);
}

#[test]
fn merge_plan_into_none_negative_path() {
    let mut merged: Option<MigrationPlan> = None;
    merge_plan(&mut merged, plan_with_db("dactests", "only"));
    let plan = merged.expect("first merge");
    assert_eq!(plan.objects.len(), 1);
    assert_eq!(plan.summary.object_count, 1);
}

#[test]
fn merge_plan_sums_summary_counts_edge_case() {
    let mut first = plan_with_db("dactests", "a");
    first.summary.create_count = 1;
    let mut second = plan_with_db("warehouse", "b");
    second.summary.create_count = 2;
    let mut merged = Some(first);
    merge_plan(&mut merged, second);
    let plan = merged.expect("merged");
    assert_eq!(plan.summary.create_count, 3);
    assert_eq!(plan.summary.object_count, 2);
}

#[test]
fn merge_plan_preserves_first_database_objects_regression() {
    let mut merged: Option<MigrationPlan> = None;
    merge_plan(&mut merged, plan_with_db("dactests", "smoke_table"));
    merge_plan(&mut merged, plan_with_db("warehouse", "fact_table"));
    let plan = merged.expect("BG-015 regression merge");
    let dbs: Vec<_> = plan
        .objects
        .iter()
        .map(|o| o.database_name.as_ref())
        .collect();
    assert!(
        dbs.contains(&"dactests"),
        "BG-015 regression: first database must remain after merge, got {dbs:?}"
    );
}
