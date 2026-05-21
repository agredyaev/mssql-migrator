use super::key::{ObjectKey, ScriptKey};
use super::shared::{empty_str, SharedStr};
use super::str_off::StrOff;

/// Trigger → parent table row id (**CASE-4** / **IDX**). Strings resolved at export/blockers only.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ParentRef {
    /// 1-based dense row id of parent table object; `0` = unknown.
    pub parent_row_id: u32,
}

/// Dense layout object row (**CASE-1** / **DOD-6**): codes + indices; strings via key / side tables.
#[derive(Clone, Debug)]
pub struct ObjectEntry {
    /// Normalized key slice in [`super::Workspace::layout_arena`] (**L1** / **IDX**).
    pub key_off: StrOff,
    /// Set during scan ingest; cleared after [`super::arena::intern_workspace_strings`].
    pub(crate) staging_key: Option<ObjectKey>,
    /// 1-based index into [`super::Workspace::scripts`].
    pub script_id: u32,
    pub checksum: [u8; 32],
    pub db_exists: bool,
    /// Index into [`super::Workspace::database_names`] (**IDX** / L2).
    pub db_id: u16,
}

impl ObjectEntry {
    /// Test / bench helper before [`super::arena::intern_workspace_strings`].
    pub fn with_staging_key(
        key: ObjectKey,
        script_id: u32,
        checksum: [u8; 32],
        db_exists: bool,
        db_id: u16,
    ) -> Self {
        Self {
            key_off: StrOff::EMPTY,
            staging_key: Some(key),
            script_id,
            checksum,
            db_exists,
            db_id,
        }
    }

    pub fn database_name<'a>(&self, ws: &'a super::Workspace) -> super::SharedStr {
        ws.database_name(self.db_id)
    }

    pub fn key<'a>(&self, ws: &'a super::Workspace) -> ObjectKey {
        if let Some(k) = &self.staging_key {
            return k.clone();
        }
        ws.object_key(self.key_off)
    }

    pub fn key_str<'a>(&'a self, ws: &'a super::Workspace) -> &'a str {
        if let Some(k) = &self.staging_key {
            return k.as_str();
        }
        ws.str_at(self.key_off)
    }

    pub fn schema_part<'a>(&'a self, ws: &'a super::Workspace) -> &'a str {
        key_part(self.key_str(ws), 0)
    }

    pub fn kind_part<'a>(&'a self, ws: &'a super::Workspace) -> &'a str {
        key_part(self.key_str(ws), 1)
    }

    pub fn name_part<'a>(&'a self, ws: &'a super::Workspace) -> &'a str {
        key_part(self.key_str(ws), 2)
    }

    pub fn schema_shared(&self, ws: &super::Workspace) -> SharedStr {
        self.key(ws).schema_shared()
    }

    pub fn kind_shared(&self, ws: &super::Workspace) -> SharedStr {
        self.key(ws).kind_shared()
    }

    pub fn name_shared(&self, ws: &super::Workspace) -> SharedStr {
        self.key(ws).name_shared()
    }

    pub fn script_path<'a>(&self, ws: &'a super::Workspace) -> &'a str {
        ws.script(self.script_id).path_str()
    }

    pub fn script_key(&self, ws: &super::Workspace) -> ScriptKey {
        ws.script(self.script_id).key()
    }

    /// Parent table row for trigger at `child_row_id` (1-based), if recorded at catalog apply.
    pub fn parent_ref_for_row<'a>(
        &'a self,
        ws: &'a super::Workspace,
        child_row_id: u32,
    ) -> Option<&'a ParentRef> {
        ws.parent_by_row.get(&child_row_id)
    }

    /// **VIEW** — materialize / JSON only.
    pub fn parent_name(&self, ws: &super::Workspace, child_row_id: u32) -> SharedStr {
        let Some(pref) = self.parent_ref_for_row(ws, child_row_id) else {
            return empty_str();
        };
        if pref.parent_row_id == 0 {
            return empty_str();
        }
        ws.entry((pref.parent_row_id as usize) - 1).name_shared(ws)
    }
}

fn key_part(key: &str, index: usize) -> &str {
    let mut parts = key.split('/');
    parts.nth(index).unwrap_or("")
}

