use crate::domain::script::{Script, ScriptGit, ScriptGitStaging};
use crate::domain::Workspace;

use super::ScriptRef;

impl Workspace {
    pub fn script_row(&self, script_id: u32) -> &crate::domain::ScriptRow {
        &self.script_rows[(script_id - 1) as usize]
    }

    pub fn script_row_mut(&mut self, script_id: u32) -> &mut crate::domain::ScriptRow {
        &mut self.script_rows[(script_id - 1) as usize]
    }

    pub fn script(&self, script_id: u32) -> ScriptRef<'_> {
        ScriptRef {
            ws: self,
            id: script_id,
        }
    }

    pub fn script_checksum(&self, script_id: u32) -> Option<&[u8; 32]> {
        self.script_checksums
            .get((script_id - 1) as usize)
            .and_then(|o| o.as_ref())
    }

    pub fn scripts_iter(&self) -> impl Iterator<Item = ScriptRef<'_>> + '_ {
        (1..=self.script_rows.len() as u32).map(move |id| self.script(id))
    }

    pub fn ensure_script_git(&mut self, script_id: u32) -> &mut ScriptGit {
        self.script_git.entry(script_id).or_default()
    }

    pub fn ensure_script_git_staging(&mut self, script_id: u32) -> &mut ScriptGitStaging {
        self.cold.script_git_staging.entry(script_id).or_default()
    }

    pub fn script_to_ingest(&self, script_id: u32) -> Script {
        let s = self.script(script_id);
        Script {
            key: s.key(),
            kind: s.kind(),
            abs_path: s.abs_path(),
            checksum: s.checksum().copied(),
            scaffold: s.scaffold(),
        }
    }
}
