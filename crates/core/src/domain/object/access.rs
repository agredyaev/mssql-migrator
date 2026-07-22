use super::super::key::ObjectKey;
use super::super::shared::SharedStr;
use super::ObjectEntry;

impl ObjectEntry {
    /// Returns the database name for this entry's database identifier.
    pub fn database_name(&self, ws: &super::super::Workspace) -> super::super::SharedStr {
        ws.database_name(self.db_id)
    }

    /// Returns the `ObjectKey` for this entry at workspace index `i`.
    pub fn key(&self, ws: &super::super::Workspace, i: usize) -> ObjectKey {
        ws.entry_key(i).clone()
    }

    /// Returns the object key string for this entry at workspace index `i`.
    pub fn key_str<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        ws.entry_key(i).as_str()
    }

    /// Returns the schema segment of the key for this entry at index `i`.
    pub fn schema_part<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        key_part(self.key_str(ws, i), 0)
    }

    /// Returns the kind segment of the key for this entry at index `i`.
    pub fn kind_part<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        key_part(self.key_str(ws, i), 1)
    }

    /// Returns the name segment of the key for this entry at index `i`.
    pub fn name_part<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        key_part(self.key_str(ws, i), 2)
    }

    /// Returns the schema segment as a `SharedStr` for this entry at index `i`.
    pub fn schema_shared(&self, ws: &super::super::Workspace, i: usize) -> SharedStr {
        self.key(ws, i).schema_shared()
    }

    /// Returns the kind segment as a `SharedStr` for this entry at index `i`.
    pub fn kind_shared(&self, ws: &super::super::Workspace, i: usize) -> SharedStr {
        self.key(ws, i).kind_shared()
    }

    /// Returns the name segment as a `SharedStr` for this entry at index `i`.
    pub fn name_shared(&self, ws: &super::super::Workspace, i: usize) -> SharedStr {
        self.key(ws, i).name_shared()
    }

    /// Returns the filesystem path of the script backing this entry.
    pub fn script_path<'a>(&self, ws: &'a super::super::Workspace) -> &'a str {
        ws.script(self.script_id).path_str()
    }
}

fn key_part(key: &str, index: usize) -> &str {
    let mut parts = key.split('/');
    parts.nth(index).unwrap_or("")
}
