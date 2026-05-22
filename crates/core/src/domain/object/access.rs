use super::super::key::{ObjectKey, ScriptKey};
use super::super::shared::SharedStr;
use super::ObjectEntry;

impl ObjectEntry {
    pub fn database_name(&self, ws: &super::super::Workspace) -> super::super::SharedStr {
        ws.database_name(self.db_id)
    }

    pub fn key(&self, ws: &super::super::Workspace, i: usize) -> ObjectKey {
        ws.entry_key(i).clone()
    }

    pub fn key_str<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        ws.entry_key(i).as_str()
    }

    pub fn schema_part<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        key_part(self.key_str(ws, i), 0)
    }

    pub fn kind_part<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        key_part(self.key_str(ws, i), 1)
    }

    pub fn name_part<'a>(&'a self, ws: &'a super::super::Workspace, i: usize) -> &'a str {
        key_part(self.key_str(ws, i), 2)
    }

    pub fn schema_shared(&self, ws: &super::super::Workspace, i: usize) -> SharedStr {
        self.key(ws, i).schema_shared()
    }

    pub fn kind_shared(&self, ws: &super::super::Workspace, i: usize) -> SharedStr {
        self.key(ws, i).kind_shared()
    }

    pub fn name_shared(&self, ws: &super::super::Workspace, i: usize) -> SharedStr {
        self.key(ws, i).name_shared()
    }

    pub fn script_path<'a>(&self, ws: &'a super::super::Workspace) -> &'a str {
        ws.script(self.script_id).path_str()
    }

    pub fn script_key(&self, ws: &super::super::Workspace) -> ScriptKey {
        ws.script(self.script_id).key()
    }
}

fn key_part(key: &str, index: usize) -> &str {
    let mut parts = key.split('/');
    parts.nth(index).unwrap_or("")
}
