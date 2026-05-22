use crate::domain::{Script, ScriptKey};

use super::Workspace;

impl Workspace {
    /// Insert or replace a script; returns 1-based [`crate::domain::ObjectEntry::script_id`].
    pub fn insert_script(&mut self, script: Script) -> u32 {
        let key = script.key.clone();
        let abs = script.abs_path.clone();
        let (row, checksum) = script.into_parts();
        if let Some(&id) = self.script_key_index.get(&key) {
            let idx = (id - 1) as usize;
            self.script_rows[idx] = row;
            self.script_checksums[idx] = checksum;
            self.cold.ingest_script_keys[idx] = key;
            self.cold.ingest_script_abs[idx] = abs;
            return id;
        }
        let id = self.script_rows.len() as u32 + 1;
        self.script_key_index.insert(key.clone(), id);
        self.script_rows.push(row);
        self.script_checksums.push(checksum);
        self.cold.ingest_script_keys.push(key);
        self.cold.ingest_script_abs.push(abs);
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
            let id = (i + 1) as u32;
            let key = self.script_rows[i].key(self, id);
            self.script_key_index.insert(key, id);
        }
    }
}
