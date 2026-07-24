use std::time::Instant;

use crate::db::{CatalogState, ChecksumMap};
use crate::domain::SchemaAction;
use crate::domain::Workspace;
use crate::error::Result;
use crate::export::{MigrationPlan, PlanSummary};
use crate::timings;

use super::diff_ctx::{DecideCtx, DiffCounters};
use super::diff_decide::decide_object_at;
use super::diff_object::{planned_object, reuse_unchanged_object};
use super::diff_plan::{ensure_plan_schemas, prepare_plan_objects};

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

/// Fills `plan` in place with a structural diff, reusing it for the same workspace layout.
pub fn compute_diff_into(
    ws: &mut Workspace,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
    plan: &mut MigrationPlan,
) -> Result<i64> {
    let t0 = Instant::now();
    if !ws.object_entries.is_empty() && ws.key_index.len() != ws.object_entries.len() {
        ws.finalize_object_layout();
    }
    crate::plan::scope::apply_catalog_if_needed(ws, catalog);
    crate::plan::scope::apply_checksums_if_needed(ws, checksums);
    let object_count = ws.object_count();
    plan.blocked = false;
    plan.blockers.clear();
    prepare_plan_objects(plan, object_count);
    ensure_plan_schemas(plan, ws.schemas.len());
    let mut counters = DiffCounters::default();

    for (i, schema) in ws.schemas.iter().enumerate() {
        let slot = &mut plan.schemas[i];
        if slot.schema_name != schema.name {
            slot.schema_name.clear();
            slot.schema_name.push_str(&schema.name);
        }
        let new_action = if catalog.schemas.contains(schema.normalized.as_str()) {
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
        let key = ws.entry_key(i);
        let kind = plan
            .objects
            .get(i)
            .filter(|object| object.normalized_key == key.as_str())
            .map(|object| object.kind.as_str())
            .unwrap_or_else(|| key.kind_part());
        let kind_code = crate::domain::kind_code(kind);
        let decision = decide_object_at(ws, i, kind_code, &mut ctx, plan);
        if let Some(slot) = plan.objects.get(i) {
            if reuse_unchanged_object(ws, i, decision, slot) {
                continue;
            }
        }
        let object = planned_object(ws, i, decision);
        if i < plan.objects.len() {
            plan.objects[i] = object;
        } else {
            plan.objects.push(object);
        }
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
