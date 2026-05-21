mod cold;
mod compact;
mod objects;
mod scripts;
mod views;

use std::ops::{Deref, DerefMut};

pub use cold::WorkspaceCold;

use super::key::ObjectKey;
use super::object::ObjectEntry;
use super::store::ObjectRow;

pub(crate) const CATALOG_APPLIED: u8 = 1 << 0;
pub(crate) const CHECKSUMS_APPLIED: u8 = 1 << 1;

/// Kelley hot shell (**W4**): dense object columns + [`WorkspaceCold`] slab.
#[derive(Clone, Debug)]
pub struct Workspace {
    pub object_entries: Vec<ObjectEntry>,
    pub(crate) object_keys: Vec<ObjectKey>,
    pub object_rows: Vec<ObjectRow>,
    pub blocked: bool,
    pub(crate) catalog_flags: u8,
    pub(crate) cold: Box<WorkspaceCold>,
}

impl Default for Workspace {
    fn default() -> Self {
        Self {
            object_entries: Vec::new(),
            object_keys: Vec::new(),
            object_rows: Vec::new(),
            blocked: false,
            catalog_flags: 0,
            cold: Box::new(WorkspaceCold::new()),
        }
    }
}

impl Deref for Workspace {
    type Target = WorkspaceCold;
    fn deref(&self) -> &WorkspaceCold {
        &self.cold
    }
}

impl DerefMut for Workspace {
    fn deref_mut(&mut self) -> &mut WorkspaceCold {
        &mut self.cold
    }
}

impl Workspace {
    pub fn row_id_at(&self, i: usize) -> u32 {
        (i + 1) as u32
    }

    pub fn insert_parent_row(&mut self, row_id: u32, parent: super::object::ParentRef) {
        self.parent_by_row.insert(row_id, parent);
    }

    pub fn script_count(&self) -> usize {
        self.script_rows.len()
    }

    pub fn row(&self, i: usize) -> &ObjectRow {
        &self.object_rows[i]
    }

    pub fn object_count(&self) -> usize {
        self.object_entries.len()
    }

    pub fn key_index(&self, key: &ObjectKey) -> u32 {
        self.cold.key_index.get(key).copied().unwrap_or(0)
    }

    /// 1-based row id for [`crate::db::key_fingerprint`] (**IDX**).
    pub fn row_id_for_fingerprint(&self, fp: u64) -> u32 {
        self.cold.fp_index.get(&fp).copied().unwrap_or(0)
    }

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
            obj.db_exists = false;
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

    pub fn reset_layout(&mut self) {
        self.object_entries.clear();
        self.object_keys.clear();
        self.object_rows.clear();
        self.cold.ingest_key_index.clear();
        self.cold.key_index.clear();
        self.cold.fp_index.clear();
        self.script_rows.clear();
        self.script_checksums.clear();
        self.script_git.clear();
        self.script_key_index.clear();
        self.transitions_by_row.clear();
        self.transitions_staging.clear();
        self.parent_by_row.clear();
        self.parent_by_object.clear();
        self.invalidate_transition_paths();
        self.schemas.clear();
        self.string_arena_bytes = 0;
        self.string_arena_unique = 0;
        self.database_names.truncate(1);
        self.layout_arena = None;
        self.invalidate_catalog_facts();
    }
}
