use std::collections::HashMap;

use crate::domain::{Action, Workspace};

use super::plan_side::{materialize_planned_git, materialize_transition_paths, PlanGitOff};
use super::{MigrationPlan, PlannedObject, PlanRow};

/// Build wire-shaped row from slim storage + layout index (**VIEW** / **DER**).
pub fn materialize_planned_object(
    ws: &Workspace,
    i: usize,
    row: &PlanRow,
    plan_git: &HashMap<u32, PlanGitOff>,
    plan_transitions: &HashMap<u32, Vec<crate::domain::StrOff>>,
) -> PlannedObject {
    let obj = ws.entry(i);
    let idx = i as u32;
    PlannedObject {
        normalized_key: obj.key(ws).shared(),
        object_path: ws.object_path_at(i),
        schema_name: obj.schema_shared(ws),
        kind: obj.kind_shared(ws),
        object_name: obj.name_shared(ws),
        database_name: obj.database_name(ws),
        parent_name: obj.parent_name(ws, ws.row_id_at(i)),
        planned_action: row.planned_action(),
        exists: row.exists(),
        checksum: row.checksum,
        git: plan_git
            .get(&idx)
            .map(|&off| materialize_planned_git(ws, off)),
        transition_paths: plan_transitions
            .get(&idx)
            .map(|offs| materialize_transition_paths(ws, offs))
            .unwrap_or_default(),
    }
}

impl MigrationPlan {
    /// Fill `objects` from `rows` + side tables (**VIEW**). No-op when `rows` empty (JSON ingest).
    pub fn ensure_objects_materialized(&mut self, ws: &Workspace) {
        if self.rows.is_empty() {
            return;
        }
        if self.objects.len() == self.rows.len() {
            return;
        }
        self.objects.clear();
        self.objects.reserve(self.rows.len());
        for i in 0..self.rows.len() {
            self.objects.push(materialize_planned_object(
                ws,
                i,
                &self.rows[i],
                &self.plan_git,
                &self.plan_transitions,
            ));
        }
    }

    pub fn row_count(&self) -> usize {
        if !self.rows.is_empty() {
            self.rows.len()
        } else {
            self.objects.len()
        }
    }

    pub fn row(&self, i: usize) -> Option<&PlanRow> {
        self.rows.get(i)
    }

    pub fn uses_slim_rows(&self) -> bool {
        !self.rows.is_empty()
    }
}

/// Wire plan for JSON serialization (**VIEW**).
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

pub fn filter_applied_migrations_on_plan(
    plan: &mut MigrationPlan,
    ws: &Workspace,
    applied: &HashMap<String, bool>,
) {
    if plan.rows.is_empty() {
        return filter_legacy_objects(plan, applied);
    }
    let need = plan.rows.iter().enumerate().any(|(i, row)| {
        row.planned_action() == Action::ReprocessChanged
            && plan
                .plan_transitions
                .get(&(i as u32))
                .is_some_and(|v| !v.is_empty())
    });
    if !need {
        return;
    }
    for (i, row) in plan.rows.iter().enumerate() {
        if row.planned_action() != Action::ReprocessChanged {
            continue;
        }
        let Some(paths) = plan.plan_transitions.get_mut(&(i as u32)) else {
            continue;
        };
        paths.retain(|off| {
            let p: &str = ws.str_at(*off);
            !applied.contains_key(p)
        });
    }
    plan.objects.clear();
}

fn filter_legacy_objects(plan: &mut MigrationPlan, applied: &HashMap<String, bool>) {
    let need = plan
        .objects
        .iter()
        .any(|o| o.planned_action == Action::ReprocessChanged && !o.transition_paths.is_empty());
    if !need {
        return;
    }
    for obj in &mut plan.objects {
        if obj.planned_action != Action::ReprocessChanged || obj.transition_paths.is_empty() {
            continue;
        }
        obj.transition_paths
            .retain(|tp| !applied.contains_key(tp.as_ref()));
    }
}
