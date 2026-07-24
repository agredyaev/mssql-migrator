use super::{ObjectKey, TransitionEntry, Workspace};

/// Trigger-to-parent table reference.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ParentRef {
    /// 1-based parent object row id; `0` means unknown.
    pub parent_row_id: u32,
}

/// One managed repository object and its planning metadata.
#[derive(Clone, Debug)]
pub struct ObjectEntry {
    /// Normalized `schema/kind/name` key.
    pub key: ObjectKey,
    /// File digest of the source script.
    pub checksum: [u8; 32],
    /// 1-based script id.
    pub script_id: u32,
    /// Database slot index.
    pub db_id: u16,
    /// Whether the object exists in SQL Server.
    pub db_exists: bool,
    /// Previously audited source checksum.
    pub prior_checksum: Option<[u8; 32]>,
    /// Parent table for trigger/index relationships.
    pub parent: Option<ParentRef>,
    /// Ordered transition scripts owned by this object.
    pub transitions: Vec<TransitionEntry>,
}

impl ObjectEntry {
    /// Creates an object entry.
    pub fn new(
        key: ObjectKey,
        script_id: u32,
        checksum: [u8; 32],
        db_exists: bool,
        db_id: u16,
    ) -> Self {
        Self {
            key,
            checksum,
            script_id,
            db_id,
            db_exists,
            prior_checksum: None,
            parent: None,
            transitions: Vec::new(),
        }
    }

    /// Returns the parent table name.
    pub fn parent_name(&self, ws: &Workspace) -> String {
        let Some(parent) = self.parent.filter(|value| value.parent_row_id > 0) else {
            return String::new();
        };
        ws.object_entries
            .get(parent.parent_row_id as usize - 1)
            .map(|entry| entry.key.name_shared())
            .unwrap_or_default()
    }
}
