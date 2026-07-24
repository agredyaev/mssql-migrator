use crate::domain::{Script, ScriptKey};

use super::Workspace;

impl Workspace {
    /// Inserts or replaces a script and returns its 1-based id.
    pub fn insert_script(&mut self, script: Script) -> u32 {
        let key = script.key.clone();
        let (row, checksum) = script.into_parts();
        if let Some(&id) = self.script_key_index.get(&key) {
            let index = id as usize - 1;
            self.script_rows[index] = row;
            self.script_checksums[index] = checksum;
            return id;
        }
        let id = self.script_rows.len() as u32 + 1;
        self.script_key_index.insert(key, id);
        self.script_rows.push(row);
        self.script_checksums.push(checksum);
        id
    }

    /// Returns the script for `key`.
    pub fn script_by_key(&self, key: &ScriptKey) -> Option<crate::domain::ScriptRef<'_>> {
        self.script_key_index
            .get(key)
            .copied()
            .map(|id| self.script(id))
    }

    /// Rebuilds the path-to-script index.
    pub fn rebuild_script_key_index(&mut self) {
        self.script_key_index = self
            .script_rows
            .iter()
            .enumerate()
            .map(|(index, row)| (row.key.clone(), index as u32 + 1))
            .collect();
    }
}
