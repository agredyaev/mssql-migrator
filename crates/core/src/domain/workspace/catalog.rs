use super::{Workspace, CATALOG_APPLIED, CHECKSUMS_APPLIED};

impl Workspace {
    /// Clears catalog facts loaded from SQL Server.
    pub fn invalidate_catalog_facts(&mut self) {
        self.catalog_flags &= !(CATALOG_APPLIED | CHECKSUMS_APPLIED);
        for object in &mut self.object_entries {
            object.db_exists = false;
            object.prior_checksum = None;
            object.parent = None;
        }
    }

    /// Returns whether the object at `index` exists in SQL Server.
    pub fn catalog_has_row(&self, index: usize) -> bool {
        self.object_entries
            .get(index)
            .is_some_and(|object| object.db_exists)
    }

    /// Returns whether the object at `index` owns transition scripts.
    pub fn row_has_transition_paths(&self, index: usize) -> bool {
        self.object_entries
            .get(index)
            .is_some_and(|object| !object.transitions.is_empty())
    }

    /// Returns whether catalog facts have been applied.
    pub fn catalog_applied(&self) -> bool {
        self.catalog_flags & CATALOG_APPLIED != 0
    }

    /// Marks catalog facts as applied.
    pub fn mark_catalog_applied(&mut self) {
        self.catalog_flags |= CATALOG_APPLIED;
    }

    /// Returns whether prior checksums have been applied.
    pub fn checksums_applied(&self) -> bool {
        self.catalog_flags & CHECKSUMS_APPLIED != 0
    }

    /// Marks prior checksums as applied.
    pub fn mark_checksums_applied(&mut self) {
        self.catalog_flags |= CHECKSUMS_APPLIED;
    }
}
