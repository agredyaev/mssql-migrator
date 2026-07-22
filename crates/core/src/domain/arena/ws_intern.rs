use super::StringArenaBuilder;

use crate::domain::Workspace;

use super::ws_apply::apply_workspace_strings;
use super::ws_register::register_workspace_strings;

/// Interns all string data from `ws` into a single compacted `StringArena`.
pub fn intern_workspace_strings(ws: &mut Workspace) {
    let n = ws.object_entries.len();
    let mut builder = StringArenaBuilder::with_capacity(
        n * 48,
        n / 4 + ws.script_rows.len() + ws.schemas.len() + 1,
    );
    register_workspace_strings(ws, &mut builder);
    install_layout_arena(ws, builder.finish());
}

/// Attach a pre-built layout arena (scan finalize or bench); skips register/build pass.
pub fn install_layout_arena(ws: &mut Workspace, arena: super::StringArena) {
    apply_workspace_strings(ws, &arena);
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
