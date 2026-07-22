use std::collections::HashMap;

use crate::domain::{ObjectEntry, Workspace};
use crate::export::plan_json::{MigrationPlan, PlannedObject};
use crate::export::plan_side::{materialize_planned_git, materialize_transition_paths, PlanGitOff};
use crate::export::PlanRow;

/// Build wire-shaped row from slim storage + layout index.
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
        normalized_key: obj.key(ws, i).shared(),
        object_path: ws.object_path_at(i),
        schema_name: obj.schema_shared(ws, i),
        kind: obj.kind_shared(ws, i),
        object_name: obj.name_shared(ws, i),
        database_name: obj.database_name(ws),
        parent_name: ObjectEntry::parent_name(ws, ws.row_id_at(i)),
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
    /// Fill `objects` from `rows` + side tables. No-op when `rows` empty (JSON ingest).
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

    /// Returns true when the plan holds compact binary rows instead of fully materialized objects.
    pub fn uses_slim_rows(&self) -> bool {
        !self.rows.is_empty()
    }
}
