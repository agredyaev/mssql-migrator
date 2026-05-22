use super::key::ObjectKey;
use super::str_off::StrOff;

mod access;
mod parent;

/// Trigger → parent table row id. Strings resolved at export/blockers only.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ParentRef {
    /// 1-based dense row id of parent table object; `0` = unknown.
    pub parent_row_id: u32,
}

pub const OBJECT_FLAG_DB_EXISTS: u8 = 1 << 0;

/// Dense object row: codes and indices; strings via key / side tables.
#[derive(Clone, Debug)]
pub struct ObjectEntry {
    pub checksum: [u8; 32],
    pub key_off: StrOff,
    pub script_id: u32,
    pub db_id: u16,
    pub flags: u8,
}

impl ObjectEntry {
    /// Test / bench helper before [`super::arena::intern_workspace_strings`].
    pub fn with_staging_key(
        key: ObjectKey,
        script_id: u32,
        checksum: [u8; 32],
        db_exists: bool,
        db_id: u16,
    ) -> (ObjectKey, Self) {
        let mut flags = 0u8;
        if db_exists {
            flags |= OBJECT_FLAG_DB_EXISTS;
        }
        (
            key,
            Self {
                key_off: StrOff::EMPTY,
                script_id,
                checksum,
                flags,
                db_id,
            },
        )
    }

    pub fn db_exists(&self) -> bool {
        self.flags & OBJECT_FLAG_DB_EXISTS != 0
    }

    pub fn set_db_exists(&mut self, exists: bool) {
        if exists {
            self.flags |= OBJECT_FLAG_DB_EXISTS;
        } else {
            self.flags &= !OBJECT_FLAG_DB_EXISTS;
        }
    }
}
