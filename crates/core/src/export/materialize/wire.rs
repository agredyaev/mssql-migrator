use crate::domain::Workspace;

use super::object::materialize_planned_object;
use crate::export::plan_json::{MigrationPlan, PlannedObject};

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

impl serde::Serialize for WireMigrationPlan<'_> {
    fn serialize<S: serde::Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        use serde::ser::SerializeStruct;
        let mut st = serializer.serialize_struct("MigrationPlan", 7)?;
        if !self.inner.command.is_empty() {
            st.serialize_field("command", &self.inner.command)?;
        }
        st.serialize_field("plannedAt", &self.inner.planned_at)?;
        st.serialize_field("blocked", &self.inner.blocked)?;
        if !self.inner.blockers.is_empty() {
            st.serialize_field("blockers", &self.inner.blockers)?;
        }
        st.serialize_field("schemas", &self.inner.schemas)?;
        st.serialize_field("objects", &self.objects)?;
        st.serialize_field("summary", &self.inner.summary)?;
        st.end()
    }
}

/// Serialize plan that already has materialized `objects` (no workspace).
pub struct PlanJsonFromObjects<'a>(pub &'a MigrationPlan);

impl serde::Serialize for PlanJsonFromObjects<'_> {
    fn serialize<S: serde::Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        use serde::ser::SerializeStruct;
        let p = self.0;
        let mut st = serializer.serialize_struct("MigrationPlan", 7)?;
        if !p.command.is_empty() {
            st.serialize_field("command", &p.command)?;
        }
        st.serialize_field("plannedAt", &p.planned_at)?;
        st.serialize_field("blocked", &p.blocked)?;
        if !p.blockers.is_empty() {
            st.serialize_field("blockers", &p.blockers)?;
        }
        st.serialize_field("schemas", &p.schemas)?;
        st.serialize_field("objects", &p.objects)?;
        st.serialize_field("summary", &p.summary)?;
        st.end()
    }
}
