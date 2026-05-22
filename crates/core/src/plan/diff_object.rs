use std::collections::HashMap;

use crate::domain::{Action, Workspace};
use crate::export::{plan_git_off_from_script, PlanGitOff, PlanRow};

use super::diff_fill_skip::skip_fill_unchanged;

pub(crate) struct ObjectDecision {
    pub action: Action,
    pub with_git: bool,
    pub exists: bool,
}

pub(crate) fn fill_plan_row(
    ws: &Workspace,
    i: usize,
    row: &mut PlanRow,
    plan_git: &mut HashMap<u32, PlanGitOff>,
    plan_transitions: &mut HashMap<u32, Vec<crate::domain::StrOff>>,
    decision: ObjectDecision,
) {
    if skip_fill_unchanged(ws, i, row, &decision) {
        return;
    }
    let obj = ws.entry(i);
    let idx = i as u32;
    row.set_planned_action(decision.action);
    row.set_exists(decision.exists);
    row.checksum = obj.checksum;

    if decision.with_git {
        if let Some(git) = plan_git_off_from_script(ws, obj.script_id) {
            plan_git.insert(idx, git);
        } else {
            plan_git.remove(&idx);
        }
    } else {
        plan_git.remove(&idx);
    }

    if decision.action == Action::ReprocessChanged {
        if let Some(p) = ws
            .transition_path_cache
            .as_ref()
            .and_then(|m| m.get(&ws.row_id_at(i)))
        {
            plan_transitions.insert(idx, p.clone());
        } else {
            plan_transitions.remove(&idx);
        }
    } else {
        plan_transitions.remove(&idx);
    }
}
