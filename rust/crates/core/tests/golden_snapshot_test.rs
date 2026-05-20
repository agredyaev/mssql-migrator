//! Compare PlanSnapshot wire shape to Go prod_gate baseline (no SQL Server).

use std::fs;
use std::path::PathBuf;

use migrator_core::gate::{read_snapshot_json, PlanSnapshot};

#[test]
fn prod_gate_baseline_json_parses() {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../internal/app/testdata/prod_gate/plan_baseline_empty_db.json");
    let data = fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {path:?}: {e}"));
    let snap: PlanSnapshot = read_snapshot_json(&data).expect("parse baseline");
    assert_eq!(snap.version, "1");
    assert!(!snap.blocked);
    assert_eq!(snap.objects.len(), 6);
    let row = &snap.objects["smoke/tables/smoke_table"];
    assert_eq!(row.planned_action, "create_object");
    assert_eq!(row.checksum_hex.len(), 64);
}
