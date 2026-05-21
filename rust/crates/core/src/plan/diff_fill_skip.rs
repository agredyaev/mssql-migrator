use crate::domain::Workspace;
use crate::export::PlanRow;

use super::diff_object::ObjectDecision;

/// **CASE-3:** warmed re-run — skip row writes when outcome unchanged.
pub(crate) fn skip_fill_unchanged(
    ws: &Workspace,
    i: usize,
    out: &PlanRow,
    decision: &ObjectDecision,
) -> bool {
    let obj = ws.entry(i);
    out.planned_action() == decision.action
        && out.exists() == decision.exists
        && out.checksum == obj.checksum
}
