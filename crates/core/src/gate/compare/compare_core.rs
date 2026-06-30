use std::collections::HashSet;

use serde::Serialize;

use crate::gate::snapshot::{PlanSnapshot, SnapshotObject};

/// Options controlling snapshot comparison behaviour.
#[derive(Debug, Clone, Default, Serialize)]
pub struct CompareOptions {
    /// Set of object keys permitted to carry plan differences in this comparison.
    pub delta_keys: HashSet<String>,
    /// Fail the comparison when plan changes appear outside the permitted delta set.
    pub strict_unexpected: bool,
}

/// Outcome of a snapshot comparison.
#[derive(Debug, Clone, Default, Serialize)]
pub struct CompareResult {
    /// Whether the comparison passed with no blocking differences detected.
    pub passed: bool,
    /// Human-readable descriptions of each detected problem.
    pub messages: Vec<String>,
    /// Keys with plan changes outside the permitted delta set.
    pub unexpected: Vec<String>,
}

/// Compares `baseline` and `current` snapshots and returns a result indicating pass/fail and any detected differences.
pub fn compare_snapshots(
    baseline: &PlanSnapshot,
    current: &PlanSnapshot,
    opts: &CompareOptions,
) -> CompareResult {
    let mut res = CompareResult {
        passed: true,
        ..Default::default()
    };
    if current.blocked && !baseline.blocked {
        res.passed = false;
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
                    res.passed = false;
                    res.messages
                        .push(format!("risky action for {key}: {}", c.planned_action));
                }
            }
            continue;
        }
        if opts.strict_unexpected {
            res.passed = false;
            res.unexpected
                .push(format!("unexpected plan change outside delta: {key}"));
        }
    }
    if !res.passed && res.messages.is_empty() {
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
