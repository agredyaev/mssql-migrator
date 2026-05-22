use crate::domain::script::ScriptRow;
use crate::domain::{ScriptKey, SharedStr, Workspace};

impl ScriptRow {
    pub fn key(&self, ws: &Workspace, script_id: u32) -> ScriptKey {
        if self.path_off.1 != 0 {
            return ScriptKey::from(ws.shared_at(self.path_off));
        }
        let idx = (script_id - 1) as usize;
        ws.cold.ingest_script_keys[idx].clone()
    }

    pub fn path_str<'a>(&'a self, ws: &'a Workspace, script_id: u32) -> &'a str {
        if self.path_off.1 != 0 {
            return ws.str_at(self.path_off);
        }
        let idx = (script_id - 1) as usize;
        ws.cold.ingest_script_keys[idx].as_str()
    }

    pub fn abs_path(&self, ws: &Workspace, script_id: u32) -> SharedStr {
        if self.abs_path_off.1 != 0 {
            return ws.shared_at(self.abs_path_off);
        }
        let idx = (script_id - 1) as usize;
        ws.cold.ingest_script_abs[idx].clone()
    }
}
