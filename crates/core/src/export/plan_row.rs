use crate::domain::Action;

/// In-memory plan row. Index `i` aligns with `Workspace::object_entries[i]`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct PlanRow {
    /// Planned action discriminant, encoded as a `u8`.
    pub action: u8,
    /// Bitfield of status flags for this row.
    pub flags: u8,
    /// Expected content checksum for this object.
    pub checksum: [u8; 32],
}

/// Flag bit set when the object already exists in the database.
pub const PLAN_FLAG_EXISTS: u8 = 1 << 0;

impl PlanRow {
    /// Returns the planned `Action` decoded from the stored discriminant.
    pub fn planned_action(self) -> Action {
        Action::from_repr(self.action).unwrap_or(Action::Fail)
    }

    /// Sets the planned action discriminant from `action`.
    pub fn set_planned_action(&mut self, action: Action) {
        self.action = action.as_repr();
    }

    /// Returns `true` if the object exists in the database.
    pub fn exists(self) -> bool {
        self.flags & PLAN_FLAG_EXISTS != 0
    }

    /// Sets or clears the exists flag.
    pub fn set_exists(&mut self, exists: bool) {
        if exists {
            self.flags |= PLAN_FLAG_EXISTS;
        } else {
            self.flags &= !PLAN_FLAG_EXISTS;
        }
    }
}
