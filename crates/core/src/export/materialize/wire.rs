use serde::Serialize;

use crate::domain::Workspace;

use super::object::materialize_planned_object;
use crate::export::plan_json::{MigrationPlan, PlanSummary, PlannedObject, PlannedSchema};

/// Wire plan for JSON serialization.
pub struct WireMigrationPlan<'a> {
    pub inner: &'a MigrationPlan,
    pub objects: Vec<PlannedObject>,
}

impl<'a> WireMigrationPlan<'a> {
    pub fn new(plan: &'a MigrationPlan, ws: &Workspace) -> Self {
        let objects = if plan.rows.is_empty() {
            plan.objects.clone()
        } else {
            (0..plan.rows.len())
                .map(|i| {
                    materialize_planned_object(
                        ws,
                        i,
                        &plan.rows[i],
                        &plan.plan_git,
                        &plan.plan_transitions,
                    )
                })
                .collect()
        };
        Self {
            inner: plan,
            objects,
        }
    }
}

/// Serialize plan that already has materialized `objects` (no workspace).
pub struct PlanJsonFromObjects<'a>(pub &'a MigrationPlan);

/// Exact wire shape: `command`/`blockers` omitted when empty, `plannedAt`
/// renamed, fields emitted in this declared order.
#[derive(Serialize)]
struct PlanWire<'a> {
    #[serde(skip_serializing_if = "str::is_empty")]
    command: &'a str,
    #[serde(rename = "plannedAt")]
    planned_at: &'a str,
    blocked: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    blockers: &'a Vec<String>,
    schemas: &'a [PlannedSchema],
    objects: &'a [PlannedObject],
    summary: &'a PlanSummary,
}

impl<'a> PlanWire<'a> {
    fn new(inner: &'a MigrationPlan, objects: &'a [PlannedObject]) -> Self {
        Self {
            command: &inner.command,
            planned_at: &inner.planned_at,
            blocked: inner.blocked,
            blockers: &inner.blockers,
            schemas: &inner.schemas,
            objects,
            summary: &inner.summary,
        }
    }
}

impl Serialize for WireMigrationPlan<'_> {
    fn serialize<S: serde::Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        PlanWire::new(self.inner, &self.objects).serialize(serializer)
    }
}

impl Serialize for PlanJsonFromObjects<'_> {
    fn serialize<S: serde::Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        PlanWire::new(self.0, &self.0.objects).serialize(serializer)
    }
}
