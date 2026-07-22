use std::fmt::Display;

mod apply;
mod blocked;
mod gate;
mod scenario;

pub use apply::compare_e2e_apply_reports;
pub use blocked::compare_e2e_blocked_reports;
pub use gate::compare_e2e_gate_reports;
pub use scenario::compare_e2e_reports;

/// Pushes a `"{name}: baseline=… actual=…"` mismatch line when the two values
/// differ, matching the hand-written per-field `format!` used across the
/// e2e-report comparators (`Display`, so scalar and string output is unchanged).
pub(super) fn diff_field<T: PartialEq + Display>(
    msgs: &mut Vec<String>,
    name: &str,
    baseline: &T,
    actual: &T,
) {
    if baseline != actual {
        msgs.push(format!("{name}: baseline={baseline} actual={actual}"));
    }
}
