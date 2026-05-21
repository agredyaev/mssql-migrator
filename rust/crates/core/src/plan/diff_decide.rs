use crate::domain::{Action, Workspace, KIND_TABLES};

use super::diff_ctx::DecideCtx;
use super::diff_object::ObjectDecision;
use crate::plan::{apply_scenario, resolve_plan_scenario, ScenarioInput};

pub(crate) fn decide_object_at(
    ws: &mut Workspace,
    i: usize,
    kind_code: u8,
    ctx: &mut DecideCtx<'_>,
) -> ObjectDecision {
    let row_id = ws.row_id_at(i);
    let has_transition_paths =
        kind_code == KIND_TABLES && ws.row_has_transition_paths(i);
    let exists = ws.entry(i).db_exists;
    let prior = ctx.plan.prior_by_row[i];
    let checksum = ws.entry(i).checksum;

    if exists {
        if let Some(p) = prior {
            if p != [0; 32] && p == checksum {
                ctx.counters.skip += 1;
                return ObjectDecision {
                    action: Action::SkipUnchanged,
                    with_git: false,
                    exists,
                };
            }
        }
        if prior.is_none() || prior == Some([0; 32]) {
            ctx.counters.adopt += 1;
            return ObjectDecision {
                action: Action::AdoptExisting,
                with_git: true,
                exists,
            };
        }
    } else {
        ctx.counters.create += 1;
        return ObjectDecision {
            action: Action::CreateObject,
            with_git: true,
            exists,
        };
    }

    let obj = ws.entry(i);
    let scenario = resolve_plan_scenario(ScenarioInput {
        exists,
        prior,
        checksum,
        kind_code,
        obj,
        ws,
        catalog: ctx.catalog,
        prior_digests: &ctx.plan.prior_by_row,
        child_row_id: row_id,
        has_transition_paths,
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
        ctx.plan.blocked = true;
    }
    let action = apply_scenario(scenario, obj, ws, row_id, &mut ctx.plan.blockers);
    ObjectDecision {
        action,
        with_git: scenario.with_git(),
        exists,
    }
}
