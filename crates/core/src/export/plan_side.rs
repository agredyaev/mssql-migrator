use crate::domain::{StrOff, Workspace};

/// Slim git side row. Materialize to [`PlannedGit`] at export only.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct PlanGitOff {
    pub hash_off: StrOff,
    pub author_off: StrOff,
    pub date_off: StrOff,
}

impl PlanGitOff {
    pub fn is_empty(self) -> bool {
        self.hash_off.1 == 0 && self.author_off.1 == 0 && self.date_off.1 == 0
    }
}

pub fn plan_git_off_from_script(ws: &Workspace, script_id: u32) -> Option<PlanGitOff> {
    let git = ws.script_git.get(&script_id)?;
    let off = PlanGitOff {
        hash_off: git.hash_off,
        author_off: git.author_off,
        date_off: git.date_off,
    };
    if off.is_empty() {
        return None;
    }
    Some(off)
}

pub fn materialize_planned_git(ws: &Workspace, off: PlanGitOff) -> super::PlannedGit {
    super::PlannedGit {
        hash: ws.shared_at(off.hash_off),
        author: ws.shared_at(off.author_off),
        date: ws.shared_at(off.date_off),
    }
}

pub fn materialize_transition_paths(
    ws: &Workspace,
    offs: &[StrOff],
) -> Vec<crate::domain::SharedStr> {
    offs.iter().map(|o| ws.shared_at(*o)).collect()
}
