use crate::domain::str_off::StrOff;
use crate::domain::Workspace;

/// Apply git staging → [`StrOff`] after [`super::intern_workspace_strings`] (preload must run before intern).
pub fn intern_script_git_strings(ws: &mut Workspace) {
    apply_script_git_offs(ws);
}

pub(super) fn apply_script_git_offs(ws: &mut Workspace) {
    let Some(arena) = ws.cold.layout_arena.as_ref() else {
        return;
    };
    for git in ws.cold.script_git.values_mut() {
        if let Some(s) = git.staging_hash.take() {
            git.hash_off = StrOff::from_arena(arena, s.as_ref());
        }
        if let Some(s) = git.staging_author.take() {
            git.author_off = StrOff::from_arena(arena, s.as_ref());
        }
        if let Some(s) = git.staging_date.take() {
            git.date_off = StrOff::from_arena(arena, s.as_ref());
        }
    }
}
