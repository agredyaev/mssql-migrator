use crate::domain::str_off::StrOff;
use crate::domain::Workspace;

/// Apply git staging → [`StrOff`] after [`super::intern_workspace_strings`] (preload must run before intern).
pub fn intern_script_git_strings(ws: &mut Workspace) {
    apply_script_git_offs(ws);
}

pub(super) fn apply_script_git_offs(ws: &mut Workspace) {
    let Some(arena) = ws.cold.layout_arena.clone() else {
        return;
    };
    let staging = std::mem::take(&mut ws.cold.script_git_staging);
    for (script_id, st) in staging {
        let git = ws.script_git.entry(script_id).or_default();
        if let Some(s) = st.hash {
            git.hash_off = StrOff::from_arena(&arena, s.as_ref());
        }
        if let Some(s) = st.author {
            git.author_off = StrOff::from_arena(&arena, s.as_ref());
        }
        if let Some(s) = st.date {
            git.date_off = StrOff::from_arena(&arena, s.as_ref());
        }
    }
}
