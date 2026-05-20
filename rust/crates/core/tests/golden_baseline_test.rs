//! Golden parity: every object in Go `plan_baseline_empty_db.json` matches Rust snapshot wire shape.

use std::fs;

use migrator_core::gate::{read_snapshot_json, PlanSnapshot, SNAPSHOT_VERSION};

#[test]
fn go_prod_gate_baseline_matches_rust_snapshot_wire() {
    let path = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../internal/app/testdata/prod_gate/plan_baseline_empty_db.json");
    let data = fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {:?}: {e}", path));
    let go: PlanSnapshot = read_snapshot_json(&data).expect("parse Go baseline");

    assert_eq!(go.version, SNAPSHOT_VERSION);
    assert!(!go.blocked);
    assert_eq!(go.objects.len(), 6);

    let round = serde_json::to_string_pretty(&go).expect("re-serialize");
    let again: PlanSnapshot = read_snapshot_json(&round).expect("roundtrip");
    assert_eq!(go, again);

    for (key, row) in &go.objects {
        assert!(!row.object_path.is_empty(), "object_path for {key}");
        assert!(!row.planned_action.is_empty());
        assert_eq!(row.checksum_hex.len(), 64, "checksum_hex for {key}");
    }
}
