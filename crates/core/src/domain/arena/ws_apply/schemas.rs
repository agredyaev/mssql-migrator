use super::StringArena;
use crate::domain::{SharedStr, Workspace};

pub(super) fn apply_transitions(ws: &mut Workspace, arena: &StringArena) {
    for entries in ws.transitions_by_row.values_mut() {
        for e in entries.iter_mut() {
            if let Some(ord) = e.staging_ord.take() {
                e.ord_off = arena
                    .str_off_if_dedicated(&ord)
                    .unwrap_or_else(|| crate::domain::StrOff::from_arena(arena, ord.as_ref()));
            }
        }
    }
}

pub(super) fn apply_schemas(ws: &mut Workspace, arena: &StringArena) {
    for schema in ws.schemas.iter_mut() {
        schema.database = remap_shared(arena, &schema.database);
        schema.name = remap_shared(arena, &schema.name);
        schema.normalized = remap_shared(arena, &schema.normalized);
    }
}

fn remap_shared(arena: &StringArena, s: &SharedStr) -> SharedStr {
    arena
        .str_off_if_dedicated(s)
        .map(|off| arena.shared_at(off.0, off.1))
        .unwrap_or_else(|| arena.get(s.as_ref()))
}
