use super::StringArenaBuilder;

use crate::domain::Workspace;

use super::ws_apply::apply_workspace_strings;
use super::ws_register::register_workspace_strings;

pub fn intern_workspace_strings(ws: &mut Workspace) {
    let n = ws.object_entries.len();
    let mut builder =
        StringArenaBuilder::with_capacity(n * 48, n / 4 + ws.script_rows.len() + ws.schemas.len() + 1);
    register_workspace_strings(ws, &mut builder);
    let arena = builder.finish();
    apply_workspace_strings(ws, &arena);
    ws.string_arena_bytes = arena.byte_len();
    ws.string_arena_unique = arena.unique_count();
    ws.layout_arena = Some(arena);
    ws.rebuild_script_key_index();
    ws.object_keys = ws
        .object_entries
        .iter()
        .map(|o| ws.object_key(o.key_off))
        .collect();
    ws.rebuild_fp_index();
    super::ws_git::apply_script_git_offs(ws);
}
