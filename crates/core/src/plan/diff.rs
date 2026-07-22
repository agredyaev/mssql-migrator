use std::time::Instant;

use crate::db::{CatalogState, ChecksumMap};
use crate::domain::SchemaAction;
use crate::domain::Workspace;
use crate::error::Result;
use crate::export::{MigrationPlan, PlanSummary};
use crate::timings;

use super::diff_ctx::{DecideCtx, DiffCounters};
use super::diff_decide::decide_object_at;
use super::diff_object::fill_plan_row;
use super::diff_plan::{ensure_plan_rows, ensure_plan_schemas};
use crate::domain::ensure_path_caches;

/// Computes a structural diff of the workspace against the live catalog and returns the plan.
pub fn compute_diff(
    ws: &mut Workspace,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
) -> Result<(MigrationPlan, i64)> {
    let mut plan = MigrationPlan::default();
    let ms = compute_diff_into(ws, catalog, checksums, &mut plan)?;
    Ok((plan, ms))
}

/// Fills `plan` in place with a structural diff of the workspace against the live catalog.
pub fn compute_diff_into(
    ws: &mut Workspace,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
    plan: &mut MigrationPlan,
) -> Result<i64> {
    let t0 = Instant::now();
    if !ws.object_entries.is_empty()
        && (ws.object_rows.is_empty() || ws.object_rows.len() != ws.object_entries.len())
    {
        ws.finalize_object_layout();
    }
    crate::plan::scope::apply_catalog_if_needed(ws, catalog);
    crate::plan::scope::apply_checksums_if_needed(ws, checksums);
    ensure_path_caches(ws);

    let object_count = ws.object_count();
    plan.blocked = false;
    plan.blockers.clear();
    ensure_plan_rows(plan, object_count);
    ensure_plan_schemas(plan, ws.schemas.len());
    let mut counters = DiffCounters::default();

    for (i, schema) in ws.schemas.iter().enumerate() {
        let slot = &mut plan.schemas[i];
        if slot.schema_name != schema.name.as_ref() {
            slot.schema_name.clear();
            slot.schema_name.push_str(schema.name.as_ref());
        }
        let new_action = if catalog.schemas.contains(schema.normalized.as_ref()) {
            SchemaAction::Exists
        } else {
            SchemaAction::CreateSchema
        };
        if slot.action != new_action {
            slot.action = new_action;
        }
    }

    let mut ctx = DecideCtx {
        checksums,
        counters: &mut counters,
    };

    for i in 0..object_count {
        let kind_code = ws.row(i).kind_code;
        let decision = decide_object_at(ws, i, kind_code, &mut ctx, plan);
        fill_plan_row(
            ws,
            i,
            &mut plan.rows[i],
            &mut plan.plan_git,
            &mut plan.plan_transitions,
            decision,
        );
    }

    plan.summary = PlanSummary {
        schema_count: ws.schemas.len(),
        object_count,
        create_count: counters.create as usize,
        adopt_count: counters.adopt as usize,
        skip_count: counters.skip as usize,
        changed_count: counters.changed as usize,
        blocked_count: counters.blocked as usize,
    };
    Ok(timings::dur_ms(t0.elapsed()))
}
