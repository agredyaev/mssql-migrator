use super::{Workspace, CATALOG_APPLIED, CHECKSUMS_APPLIED, WORKSPACE_FLAG_BLOCKED};

impl Workspace {
    pub fn invalidate_transition_paths(&mut self) {
        self.transition_path_cache = None;
        self.object_path_cache = None;
        self.has_transition_paths_row.clear();
    }

    pub fn invalidate_catalog_facts(&mut self) {
        self.catalog_flags &= !(CATALOG_APPLIED | CHECKSUMS_APPLIED);
        self.parent_by_object.clear();
        self.parent_by_row.clear();
        self.prior_by_row.clear();
        self.catalog_row.clear();
        for obj in &mut self.object_entries {
            obj.set_db_exists(false);
        }
    }

    #[inline]
    pub fn prior_digest(&self, i: usize) -> Option<[u8; 32]> {
        self.prior_by_row.get(i).and_then(|o| *o)
    }

    #[inline]
    pub fn catalog_has_row(&self, row_index: usize) -> bool {
        self.catalog_row.get(row_index).copied().unwrap_or(0) != 0
    }

    #[inline]
    pub fn row_has_transition_paths(&self, row_index: usize) -> bool {
        self.has_transition_paths_row
            .get(row_index)
            .copied()
            .unwrap_or(0)
            != 0
    }

    pub fn catalog_applied(&self) -> bool {
        self.catalog_flags & CATALOG_APPLIED != 0
    }

    pub fn mark_catalog_applied(&mut self) {
        self.catalog_flags |= CATALOG_APPLIED;
    }

    pub fn checksums_applied(&self) -> bool {
        self.catalog_flags & CHECKSUMS_APPLIED != 0
    }

    pub fn mark_checksums_applied(&mut self) {
        self.catalog_flags |= CHECKSUMS_APPLIED;
    }

    #[inline]
    pub fn blocked(&self) -> bool {
        self.catalog_flags & WORKSPACE_FLAG_BLOCKED != 0
    }

    #[inline]
    pub fn set_blocked(&mut self, on: bool) {
        if on {
            self.catalog_flags |= WORKSPACE_FLAG_BLOCKED;
        } else {
            self.catalog_flags &= !WORKSPACE_FLAG_BLOCKED;
        }
    }
}
