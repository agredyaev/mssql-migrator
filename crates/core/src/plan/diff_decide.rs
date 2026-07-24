use crate::domain::{Workspace, KIND_TABLES};

use super::diff_ctx::DecideCtx;
use super::diff_object::ObjectDecision;
use crate::plan::{apply_scenario, resolve_plan_scenario, ScenarioInput};

pub(crate) fn decide_object_at(
    ws: &mut Workspace,
    i: usize,
    kind_code: u8,
    ctx: &mut DecideCtx<'_>,
    plan: &mut crate::export::MigrationPlan,
) -> ObjectDecision {
    let exists = ws.entry(i).db_exists;
    let prior = ws.entry(i).prior_checksum;
    let checksum = ws.entry(i).checksum;

    let has_transition_paths = kind_code == KIND_TABLES && ws.row_has_transition_paths(i);
    let obj = ws.entry(i);
    let scenario = resolve_plan_scenario(ScenarioInput {
        exists,
        prior,
        checksum,
        kind_code,
        obj,
        ws,
        has_transition_paths,
        live_definition_drift: ctx.checksums.has_live_definition_drift(ws.entry_key(i)),
    });
    let c = &mut ctx.counters;
    match scenario.counter_kind() {
        super::scenario::CounterKind::Create => c.create += 1,
        super::scenario::CounterKind::Adopt => c.adopt += 1,
        super::scenario::CounterKind::Skip => c.skip += 1,
        super::scenario::CounterKind::Changed => c.changed += 1,
    }
    let blocked_inc = scenario.blocked_delta();
    c.blocked += blocked_inc as i64;
    if blocked_inc > 0 {
        plan.blocked = true;
    }
    let action = apply_scenario(scenario, obj, ws, &mut plan.blockers);
    ObjectDecision {
        action,
        with_git: scenario.with_git(),
        exists,
    }
}
