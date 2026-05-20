use crate::domain::{Action, SharedStr, Workspace};

pub(crate) struct ObjectDecision {
    pub action: Action,
    pub tpaths: Vec<SharedStr>,
    pub with_git: bool,
    pub exists: bool,
}

pub(crate) fn fill_planned_at(
    ws: &Workspace,
    i: usize,
    out: &mut crate::export::PlannedObject,
    decision: ObjectDecision,
) {
    let obj = ws.entry(i);
    out.normalized_key = obj.key.shared();
    out.object_path = obj.script.shared();
    out.schema_name = obj.schema.clone();
    out.kind = obj.kind.clone();
    out.object_name = obj.name.clone();
    out.database_name = obj.database_name.clone();
    out.parent_name = obj.parent_name.clone();
    out.planned_action = decision.action;
    out.exists = decision.exists;
    out.checksum = obj.checksum;
    out.git = if decision.with_git {
        git_from_script(ws, &obj.script)
    } else {
        None
    };
    if decision.action == Action::ReprocessChanged {
        if let Some(p) = ws
            .transition_path_cache
            .as_ref()
            .and_then(|m| m.get(&obj.key))
        {
            if out.transition_paths.len() == p.len() {
                for (slot, path) in out.transition_paths.iter_mut().zip(p.iter()) {
                    *slot = path.clone();
                }
            } else {
                out.transition_paths.clone_from(p);
            }
        } else {
            out.transition_paths.clear();
        }
    } else if decision.tpaths.is_empty() {
        out.transition_paths.clear();
    } else {
        out.transition_paths = decision.tpaths;
    }
}

pub(crate) fn git_from_script(
    ws: &Workspace,
    key: &crate::domain::ScriptKey,
) -> Option<crate::export::PlannedGit> {
    let s = ws.scripts.get(key)?;
    if s.git_hash.is_empty() && s.git_author.is_empty() && s.git_date.is_empty() {
        return None;
    }
    Some(crate::export::PlannedGit {
        hash: s.git_hash.clone(),
        author: s.git_author.clone(),
        date: s.git_date.clone(),
    })
}
