use serde::Serialize;

use crate::export::plan_json::{MigrationPlan, PlanSummary, PlannedObject, PlannedSchema};

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

impl Serialize for PlanJsonFromObjects<'_> {
    fn serialize<S: serde::Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        PlanWire::new(self.0, &self.0.objects).serialize(serializer)
    }
}
