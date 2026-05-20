use std::time::Instant;

use crate::db::{CatalogState, ChecksumMap};
use crate::domain::{empty_str, Action, Workspace};
use crate::error::Result;
use crate::export::{MigrationPlan, PlanSummary, PlannedObject, PlannedSchema};
use crate::timings;

use super::diff_ctx::{DecideCtx, DiffCounters};
use super::diff_decide::decide_object_at;
use super::diff_object::fill_planned_at;
use super::transitions;

pub fn compute_diff(
    ws: &mut Workspace,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
) -> Result<(MigrationPlan, i64)> {
    let mut plan = MigrationPlan::default();
    let ms = compute_diff_into(ws, catalog, checksums, &mut plan)?;
    Ok((plan, ms))
}

pub fn compute_diff_into(
    ws: &mut Workspace,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
    plan: &mut MigrationPlan,
) -> Result<i64> {
    let t0 = Instant::now();
    ws.blocked = false;
    if !ws.object_entries.is_empty()
        && (ws.object_store.is_empty() || ws.object_store.len() != ws.object_entries.len())
    {
        ws.finalize_object_layout();
    }
    crate::plan::scope::apply_catalog_if_needed(ws, catalog);
    crate::plan::scope::apply_checksums_if_needed(ws, checksums);
    if !ws.transitions_by_table.is_empty() && ws.transition_path_cache.is_none() {
        ws.transition_path_cache = Some(transitions::paths_by_table(ws));
    }

    let object_count = ws.object_count();
    plan.blocked = false;
    plan.blockers.clear();
    ensure_plan_objects(plan, object_count);
    ensure_plan_schemas(plan, ws.schemas.len());
    let mut counters = DiffCounters::default();

    for (i, schema) in ws.schemas.iter().enumerate() {
        let slot = &mut plan.schemas[i];
        if slot.schema_name != schema.name.as_ref() {
            slot.schema_name.clear();
            slot.schema_name.push_str(schema.name.as_ref());
        }
        slot.action = if catalog.schemas.contains(schema.normalized.as_ref()) {
            crate::domain::SchemaAction::Exists
        } else {
            crate::domain::SchemaAction::CreateSchema
        };
    }

    for i in 0..object_count {
        let kind_code = ws.object_store.row(i).kind_code;
        let mut ctx = DecideCtx {
            catalog,
            checksums,
            plan,
            counters: &mut counters,
        };
        let decision = decide_object_at(ws, i, kind_code, &mut ctx);
        fill_planned_at(ws, i, &mut plan.objects[i], decision);
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

fn ensure_plan_objects(plan: &mut MigrationPlan, n: usize) {
    if plan.objects.capacity() < n {
        plan.objects.reserve(n - plan.objects.capacity());
    }
    if plan.objects.len() < n {
        plan.objects.resize_with(n, empty_planned_object);
    } else {
        plan.objects.truncate(n);
    }
}

fn ensure_plan_schemas(plan: &mut MigrationPlan, n: usize) {
    if plan.schemas.capacity() < n {
        plan.schemas.reserve(n - plan.schemas.capacity());
    }
    if plan.schemas.len() < n {
        plan.schemas.resize_with(n, || PlannedSchema {
            schema_name: String::new(),
            action: crate::domain::SchemaAction::Exists,
        });
    } else {
        plan.schemas.truncate(n);
    }
}

fn empty_planned_object() -> PlannedObject {
    PlannedObject {
        normalized_key: empty_str(),
        object_path: empty_str(),
        schema_name: empty_str(),
        kind: empty_str(),
        object_name: empty_str(),
        database_name: empty_str(),
        parent_name: empty_str(),
        planned_action: Action::SkipUnchanged,
        exists: false,
        checksum: [0; 32],
        git: None,
        transition_paths: Vec::new(),
    }
}
