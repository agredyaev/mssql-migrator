use crate::domain::{Workspace, KIND_TABLES};

use super::diff_ctx::DecideCtx;
use super::diff_object::ObjectDecision;
use super::scenario::{apply_scenario, resolve_plan_scenario};

pub(crate) fn decide_object_at(
    ws: &mut Workspace,
    i: usize,
    kind_code: u8,
    ctx: &mut DecideCtx<'_>,
) -> ObjectDecision {
    let key = ws.entry(i).key.clone();
    let has_transition_paths = kind_code == KIND_TABLES
        && ws
            .transition_path_cache
            .as_ref()
            .and_then(|m| m.get(&key))
            .is_some_and(|v| !v.is_empty());
    let obj = ws.entry_mut(i);
    let exists = obj.db.exists;
    let prior = ctx.checksums.get(&key).copied();
    let checksum = obj.checksum;
    let scenario = resolve_plan_scenario(
        exists,
        prior,
        checksum,
        kind_code,
        obj,
        ctx.catalog,
        ctx.checksums,
        has_transition_paths,
    );
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
    let action = apply_scenario(scenario, obj, &mut ctx.plan.blockers);
    ObjectDecision {
        action,
        tpaths: Vec::new(),
        with_git: scenario.with_git(),
        exists,
    }
}
