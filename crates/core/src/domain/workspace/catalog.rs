use super::{Workspace, CATALOG_APPLIED, CHECKSUMS_APPLIED, WORKSPACE_FLAG_BLOCKED};

impl Workspace {
    /// Clears cached transition-path data, forcing a reload on next access.
    pub fn invalidate_transition_paths(&mut self) {
        self.transition_path_cache = None;
        self.object_path_cache = None;
        self.has_transition_paths_row.clear();
    }

    /// Resets all catalog facts loaded from the database.
    pub fn invalidate_catalog_facts(&mut self) {
        self.catalog_flags &= !(CATALOG_APPLIED | CHECKSUMS_APPLIED);
        self.parent_by_row.clear();
        self.prior_by_row.clear();
        self.catalog_row.clear();
        for obj in &mut self.object_entries {
            obj.set_db_exists(false);
        }
    }

    #[inline]
    /// Returns the recorded prior digest for the row at index `i`, if any.
    pub fn prior_digest(&self, i: usize) -> Option<[u8; 32]> {
        self.prior_by_row.get(i).and_then(|o| *o)
    }

    #[inline]
    /// Returns `true` when the catalog contains a row at `row_index`.
    pub fn catalog_has_row(&self, row_index: usize) -> bool {
        self.catalog_row.get(row_index).copied().unwrap_or(0) != 0
    }

    #[inline]
    /// Returns `true` when transition-path rows exist for `row_index`.
    pub fn row_has_transition_paths(&self, row_index: usize) -> bool {
        self.has_transition_paths_row
            .get(row_index)
            .copied()
            .unwrap_or(0)
            != 0
    }

    /// Returns `true` when catalog facts have been applied to the workspace.
    pub fn catalog_applied(&self) -> bool {
        self.catalog_flags & CATALOG_APPLIED != 0
    }

    /// Marks catalog facts as applied.
    pub fn mark_catalog_applied(&mut self) {
        self.catalog_flags |= CATALOG_APPLIED;
    }

    /// Returns `true` when object checksums have been applied to the workspace.
    pub fn checksums_applied(&self) -> bool {
        self.catalog_flags & CHECKSUMS_APPLIED != 0
    }

    /// Marks object checksums as applied.
    pub fn mark_checksums_applied(&mut self) {
        self.catalog_flags |= CHECKSUMS_APPLIED;
    }

    #[inline]
    /// Returns `true` when the workspace is in a blocked state.
    pub fn blocked(&self) -> bool {
        self.catalog_flags & WORKSPACE_FLAG_BLOCKED != 0
    }

    #[inline]
    /// Sets or clears the blocked flag on the workspace.
    pub fn set_blocked(&mut self, on: bool) {
        if on {
            self.catalog_flags |= WORKSPACE_FLAG_BLOCKED;
        } else {
            self.catalog_flags &= !WORKSPACE_FLAG_BLOCKED;
        }
    }
}
