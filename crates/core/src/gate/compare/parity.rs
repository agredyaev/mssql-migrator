use std::collections::HashSet;

use crate::gate::snapshot::{PlanSnapshot, SnapshotObject};

/// Full equality check for e2e baselines (all object keys and wire fields).
pub fn parity_messages(expected: &PlanSnapshot, actual: &PlanSnapshot) -> Vec<String> {
    let mut msgs = Vec::new();
    if expected.version != actual.version {
        msgs.push(format!(
            "version: baseline={} actual={}",
            expected.version, actual.version
        ));
    }
    if expected.blocked != actual.blocked {
        msgs.push(format!(
            "blocked: baseline={} actual={}",
            expected.blocked, actual.blocked
        ));
    }
    let mut keys: HashSet<_> = expected.objects.keys().cloned().collect();
    keys.extend(actual.objects.keys().cloned());
    for key in keys {
        match (expected.objects.get(&key), actual.objects.get(&key)) {
            (None, Some(_)) => msgs.push(format!("missing in baseline snapshot: {key}")),
            (Some(_), None) => msgs.push(format!("missing in actual snapshot: {key}")),
            (Some(e), Some(a)) => parity_object_fields(&key, e, a, &mut msgs),
            (None, None) => {}
        }
    }
    msgs
}

fn parity_object_fields(key: &str, e: &SnapshotObject, a: &SnapshotObject, msgs: &mut Vec<String>) {
    if e.object_path != a.object_path {
        msgs.push(format!(
            "{key} object_path: baseline={} actual={}",
            e.object_path, a.object_path
        ));
    }
    if e.planned_action != a.planned_action {
        msgs.push(format!(
            "{key} planned_action: baseline={} actual={}",
            e.planned_action, a.planned_action
        ));
    }
    if e.checksum_hex != a.checksum_hex {
        msgs.push(format!(
            "{key} checksum_hex: baseline={} actual={}",
            e.checksum_hex, a.checksum_hex
        ));
    }
    if e.exists != a.exists {
        msgs.push(format!(
            "{key} exists: baseline={} actual={}",
            e.exists, a.exists
        ));
    }
}
