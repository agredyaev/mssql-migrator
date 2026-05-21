use crate::domain::Action;

/// Hot in-memory plan row (**SLAB** / **TAG**). Index `i` aligns with `Workspace::object_entries[i]`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct PlanRow {
    pub action: u8,
    pub flags: u8,
    pub checksum: [u8; 32],
}

pub const PLAN_FLAG_EXISTS: u8 = 1 << 0;

impl PlanRow {
    pub fn planned_action(self) -> Action {
        Action::from_repr(self.action).unwrap_or(Action::Fail)
    }

    pub fn set_planned_action(&mut self, action: Action) {
        self.action = action.as_repr();
    }

    pub fn exists(self) -> bool {
        self.flags & PLAN_FLAG_EXISTS != 0
    }

    pub fn set_exists(&mut self, exists: bool) {
        if exists {
            self.flags |= PLAN_FLAG_EXISTS;
        } else {
            self.flags &= !PLAN_FLAG_EXISTS;
        }
    }
}
