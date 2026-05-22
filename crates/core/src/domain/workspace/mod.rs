mod cold;
mod compact;
mod objects;
mod scripts;
mod views;

mod catalog;
mod reset;

use std::ops::{Deref, DerefMut};

pub use cold::WorkspaceCold;

use super::key::ObjectKey;
use super::object::ObjectEntry;
use super::store::ObjectRow;

pub(crate) const CATALOG_APPLIED: u8 = 1 << 0;
pub(crate) const CHECKSUMS_APPLIED: u8 = 1 << 1;
pub(crate) const WORKSPACE_FLAG_BLOCKED: u8 = 1 << 2;

/// Hot workspace shell: dense object columns + [`WorkspaceCold`] side store.
#[derive(Clone, Debug)]
pub struct Workspace {
    pub object_entries: Vec<ObjectEntry>,
    pub(crate) object_keys: Vec<ObjectKey>,
    pub object_rows: Vec<ObjectRow>,
    pub(crate) cold: Box<WorkspaceCold>,
    pub(crate) catalog_flags: u8,
}

impl Default for Workspace {
    fn default() -> Self {
        Self {
            object_entries: Vec::new(),
            object_keys: Vec::new(),
            object_rows: Vec::new(),
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

    /// 1-based row id for [`crate::domain::key_fingerprint`].
    pub fn row_id_for_fingerprint(&self, fp: u64) -> u32 {
        self.cold.fp_index.get(&fp).copied().unwrap_or(0)
    }
}
