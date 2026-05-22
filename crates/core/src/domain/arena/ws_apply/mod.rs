use super::StringArena;
use crate::domain::Workspace;

pub(super) fn apply_workspace_strings(ws: &mut Workspace, arena: &StringArena) {
    entries::apply_object_entries(ws, arena);
    apply_parents(ws, arena);
    scripts::apply_scripts(ws, arena);
    schemas::apply_transitions(ws, arena);
    ws.invalidate_transition_paths();
    schemas::apply_schemas(ws, arena);
}

fn apply_parents(_ws: &mut Workspace, _arena: &StringArena) {
    // ParentRef is row-id only; no string fields to intern.
}

mod entries;
mod schemas;
mod scripts;
