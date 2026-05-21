use crate::domain::{Script, ScriptKey};

use super::Workspace;

impl Workspace {
    /// Insert or replace a script; returns 1-based [`crate::domain::ObjectEntry::script_id`].
    pub fn insert_script(&mut self, script: Script) -> u32 {
        let key = script.key.clone();
        let (row, checksum) = script.into_parts();
        if let Some(&id) = self.script_key_index.get(&key) {
            let idx = (id - 1) as usize;
            self.script_rows[idx] = row;
            self.script_checksums[idx] = checksum;
            return id;
        }
        let id = self.script_rows.len() as u32 + 1;
        self.script_key_index.insert(key, id);
        self.script_rows.push(row);
        self.script_checksums.push(checksum);
        id
    }

    pub fn script_by_key(&self, key: &ScriptKey) -> Option<crate::domain::ScriptRef<'_>> {
        let id = self.script_key_index.get(key).copied()?;
        Some(self.script(id))
    }

    pub fn rebuild_script_key_index(&mut self) {
        self.script_key_index.clear();
        let n = self.script_rows.len();
        for i in 0..n {
            let key = self.script_rows[i].key(self);
            self.script_key_index.insert(key, (i + 1) as u32);
        }
    }
}
