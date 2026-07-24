use migrator_core::db::{CatalogState, ChecksumMap};
use migrator_core::domain::Workspace;
use migrator_core::plan::compute_diff;
use migrator_core::scan::scan_root;
use std::path::Path;

fn write_multi_db_layout(root: &Path) {
    std::fs::create_dir_all(root.join("dactests/smoke/tables")).unwrap();
    std::fs::create_dir_all(root.join("dactests/smoke/views")).unwrap();
    std::fs::create_dir_all(root.join("warehouse/reporting/tables")).unwrap();
    std::fs::write(
        root.join("dactests/smoke/tables/smoke_table.sql"),
        "CREATE TABLE smoke.smoke_table(id INT NOT NULL);\n",
    )
    .unwrap();
    std::fs::write(
        root.join("dactests/smoke/views/smoke_view.sql"),
        "CREATE VIEW smoke.smoke_view AS SELECT id FROM smoke.smoke_table;\n",
    )
    .unwrap();
    std::fs::write(
        root.join("warehouse/reporting/tables/fact_table.sql"),
        "CREATE TABLE reporting.fact_table(id INT NOT NULL);\n",
    )
    .unwrap();
}

fn workspace_from_layout(root: &Path) -> Workspace {
    let mut ws = Workspace::default();
    scan_root(&mut ws, root.to_str().unwrap()).unwrap();
    ws
}

#[test]
fn catalog_subset_materialization_happy_path() {
    let base = tempfile::tempdir().unwrap();
    write_multi_db_layout(base.path());
    let ws = workspace_from_layout(base.path());

    for db in ["dactests", "warehouse"] {
        let mut sub = ws.for_catalog_database(db);
        let (plan, _) =
            compute_diff(&mut sub, &CatalogState::default(), &ChecksumMap::new()).unwrap();
        assert_eq!(plan.objects.len(), sub.object_count());
        assert!(plan.objects.iter().all(|o| o.database_name == db));
    }
}

#[test]
fn empty_catalog_produces_create_actions_negative_path() {
    let base = tempfile::tempdir().unwrap();
    write_multi_db_layout(base.path());
    let ws = workspace_from_layout(base.path());
    let mut sub = ws.for_catalog_database("dactests");
    let (plan, _) = compute_diff(&mut sub, &CatalogState::default(), &ChecksumMap::new()).unwrap();
    assert!(
        plan.summary.create_count > 0,
        "empty catalog should plan create actions"
    );
    assert_eq!(plan.summary.object_count, sub.object_count());
}

#[test]
fn single_object_database_subset_edge_case() {
    let base = tempfile::tempdir().unwrap();
    std::fs::create_dir_all(base.path().join("warehouse/reporting/tables")).unwrap();
    std::fs::write(
        base.path()
            .join("warehouse/reporting/tables/fact_table.sql"),
        "CREATE TABLE reporting.fact_table(id INT NOT NULL);\n",
    )
    .unwrap();
    let ws = workspace_from_layout(base.path());
    let mut sub = ws.for_catalog_database("warehouse");
    let (plan, _) = compute_diff(&mut sub, &CatalogState::default(), &ChecksumMap::new()).unwrap();
    assert_eq!(plan.objects.len(), 1);
    assert_eq!(plan.objects[0].database_name, "warehouse");
}

#[test]
fn first_database_objects_survive_subset_regression() {
    let base = tempfile::tempdir().unwrap();
    write_multi_db_layout(base.path());
    let ws = workspace_from_layout(base.path());
    let mut sub = ws.for_catalog_database("dactests");
    let (plan, _) = compute_diff(&mut sub, &CatalogState::default(), &ChecksumMap::new()).unwrap();
    let keys: Vec<_> = plan
        .objects
        .iter()
        .map(|o| o.normalized_key.clone())
        .collect();
    assert!(
        keys.iter().any(|k| k.contains("smoke_table")),
        "BG-015 regression: dactests subset must keep smoke_table, got {keys:?}"
    );
    assert!(
        keys.iter().any(|k| k.contains("smoke_view")),
        "BG-015 regression: dactests subset must keep smoke_view, got {keys:?}"
    );
}
