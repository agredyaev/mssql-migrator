use std::collections::HashMap;

use migrator_core::domain::{Action, Workspace};
use migrator_core::export::{MigrationPlan, PlannedObject};
use migrator_core::plan::filter_migrations;

#[test]
fn drops_already_applied_transition_paths() {
    let mut plan = MigrationPlan::default();
    plan.objects = vec![PlannedObject {
            normalized_key: "r/tables/t1".into(),
            object_path: "r/tables/t1.sql".into(),
            schema_name: "r".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            planned_action: Action::ReprocessChanged,
            exists: true,
            checksum: [1; 32],
            transition_paths: vec![
                "r/tables/_migrations/t1/001_abc_def.sql".into(),
                "r/tables/_migrations/t1/002_abc_ghi.sql".into(),
            ],
            database_name: Default::default(),
            parent_name: Default::default(),
            git: None,
        }];
    let mut applied = HashMap::new();
    applied.insert("r/tables/_migrations/t1/001_abc_def.sql".into(), true);
    filter_migrations::filter_applied_migrations(&mut plan, &Workspace::default(), &applied);
    assert_eq!(plan.objects[0].transition_paths.len(), 1);
    assert!(plan.objects[0].transition_paths[0].contains("002"));
}
