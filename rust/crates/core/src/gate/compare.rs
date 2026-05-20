use std::collections::HashSet;

use serde::Serialize;

use super::snapshot::{PlanSnapshot, SnapshotObject};

#[derive(Debug, Clone, Default, Serialize)]
pub struct CompareOptions {
    pub delta_keys: HashSet<String>,
    pub strict_unexpected: bool,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct CompareResult {
    pub go: bool,
    pub messages: Vec<String>,
    pub unexpected: Vec<String>,
}

pub fn compare_snapshots(
    baseline: &PlanSnapshot,
    current: &PlanSnapshot,
    opts: &CompareOptions,
) -> CompareResult {
    let mut res = CompareResult {
        go: true,
        ..Default::default()
    };
    if current.blocked && !baseline.blocked {
        res.go = false;
        res.messages.push("plan became blocked".into());
    }
    let mut keys: HashSet<_> = baseline.objects.keys().cloned().collect();
    keys.extend(current.objects.keys().cloned());
    for key in keys {
        let diffs = diff_entry(baseline.objects.get(&key), current.objects.get(&key));
        if diffs.is_empty() {
            continue;
        }
        let in_delta = opts.delta_keys.is_empty() || opts.delta_keys.contains(&key);
        if in_delta {
            if let Some(c) = current.objects.get(&key) {
                if is_risky_action(&c.planned_action) {
                    res.go = false;
                    res.messages.push(format!("risky action for {key}: {}", c.planned_action));
                }
            }
            continue;
        }
        if opts.strict_unexpected {
            res.go = false;
            res.unexpected.push(format!("unexpected plan change outside delta: {key}"));
        }
    }
    if !res.go && res.messages.is_empty() {
        res.messages.push("incremental plan gate failed".into());
    }
    res
}

fn diff_entry(b: Option<&SnapshotObject>, c: Option<&SnapshotObject>) -> String {
    match (b, c) {
        (None, None) => String::new(),
        (None, Some(c)) => c.planned_action.clone(),
        (Some(b), None) => b.planned_action.clone(),
        (Some(b), Some(c)) => {
            if b.planned_action != c.planned_action {
                return c.planned_action.clone();
            }
            if b.checksum_hex != c.checksum_hex {
                return "checksum".into();
            }
            if b.exists != c.exists {
                return "exists".into();
            }
            String::new()
        }
    }
}

fn is_risky_action(action: &str) -> bool {
    matches!(action, "fail" | "reprocess_changed_blocked")
}

pub fn read_snapshot_json(data: &str) -> Result<PlanSnapshot, serde_json::Error> {
    serde_json::from_str(data)
}

pub fn write_snapshot_json(snap: &PlanSnapshot) -> Result<String, serde_json::Error> {
    serde_json::to_string_pretty(snap)
}

pub fn write_snapshot_file(path: &std::path::Path, snap: &PlanSnapshot) -> std::io::Result<()> {
    let data = write_snapshot_json(snap).map_err(std::io::Error::other)?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    std::fs::write(path, format!("{data}\n"))
}

/// Full equality check for Go↔Rust e2e parity (all object keys and wire fields).
pub fn parity_messages(expected: &PlanSnapshot, actual: &PlanSnapshot) -> Vec<String> {
    let mut msgs = Vec::new();
    if expected.version != actual.version {
        msgs.push(format!(
            "version: go={} rust={}",
            expected.version, actual.version
        ));
    }
    if expected.blocked != actual.blocked {
        msgs.push(format!(
            "blocked: go={} rust={}",
            expected.blocked, actual.blocked
        ));
    }
    let mut keys: HashSet<_> = expected.objects.keys().cloned().collect();
    keys.extend(actual.objects.keys().cloned());
    for key in keys {
        match (expected.objects.get(&key), actual.objects.get(&key)) {
            (None, Some(_)) => msgs.push(format!("missing in go snapshot: {key}")),
            (Some(_), None) => msgs.push(format!("missing in rust snapshot: {key}")),
            (Some(e), Some(a)) => {
                if e.object_path != a.object_path {
                    msgs.push(format!(
                        "{key} object_path: go={} rust={}",
                        e.object_path, a.object_path
                    ));
                }
                if e.planned_action != a.planned_action {
                    msgs.push(format!(
                        "{key} planned_action: go={} rust={}",
                        e.planned_action, a.planned_action
                    ));
                }
                if e.checksum_hex != a.checksum_hex {
                    msgs.push(format!(
                        "{key} checksum_hex: go={} rust={}",
                        e.checksum_hex, a.checksum_hex
                    ));
                }
                if e.exists != a.exists {
                    msgs.push(format!(
                        "{key} exists: go={} rust={}",
                        e.exists, a.exists
                    ));
                }
            }
            (None, None) => {}
        }
    }
    msgs
}
