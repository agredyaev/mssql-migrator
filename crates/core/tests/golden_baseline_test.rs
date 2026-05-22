//! Golden parity: every object in `plan_baseline_empty_db.json` matches snapshot wire shape.

use migrator_core::gate::{read_snapshot_json, PlanSnapshot, SNAPSHOT_VERSION};

#[test]
fn baseline_empty_db_roundtrip() {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("tests/testdata/prod_gate/plan_baseline_empty_db.json");
    let data = std::fs::read_to_string(&path).expect("read baseline");
    let baseline: PlanSnapshot = read_snapshot_json(&data).expect("parse baseline snapshot");

    assert_eq!(baseline.version, SNAPSHOT_VERSION);
    assert!(!baseline.blocked);
    assert_eq!(baseline.objects.len(), 6);

    let again: PlanSnapshot =
        read_snapshot_json(&serde_json::to_string(&baseline).unwrap()).expect("roundtrip");
    assert_eq!(baseline, again);

    for (key, row) in &baseline.objects {
        assert!(!key.is_empty(), "object key");
        assert!(!row.planned_action.is_empty(), "{key} planned_action");
        assert_eq!(row.checksum_hex.len(), 64, "{key} checksum_hex");
    }
}
