//! Compare `PlanSnapshot` wire shape to prod_gate baseline (no SQL Server).

use migrator_core::gate::{read_snapshot_json, PlanSnapshot, SNAPSHOT_VERSION};

#[test]
fn prod_gate_baseline_shape() {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("tests/testdata/prod_gate/plan_baseline_empty_db.json");
    let data = std::fs::read_to_string(&path).expect("read baseline");
    let snap: PlanSnapshot = read_snapshot_json(&data).expect("parse baseline");

    assert_eq!(snap.version, SNAPSHOT_VERSION);
    assert!(!snap.blocked);
    assert_eq!(snap.objects.len(), 6);
}
