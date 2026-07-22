use crate::domain::script::{Script, ScriptGitStaging};
use crate::domain::Workspace;

use super::ScriptRef;

impl Workspace {
    /// Returns the `ScriptRow` for `script_id` (1-based).
    pub fn script_row(&self, script_id: u32) -> &crate::domain::ScriptRow {
        &self.script_rows[(script_id - 1) as usize]
    }

    /// Returns a `ScriptRef` view for `script_id` (1-based).
    pub fn script(&self, script_id: u32) -> ScriptRef<'_> {
        ScriptRef {
            ws: self,
            id: script_id,
        }
    }

    /// Returns the stored checksum for `script_id`, or `None` if not recorded.
    pub fn script_checksum(&self, script_id: u32) -> Option<&[u8; 32]> {
        self.script_checksums
            .get((script_id - 1) as usize)
            .and_then(|o| o.as_ref())
    }

    /// Returns an iterator over all scripts in the workspace as `ScriptRef` views.
    pub fn scripts_iter(&self) -> impl Iterator<Item = ScriptRef<'_>> + '_ {
        (1..=self.script_rows.len() as u32).map(move |id| self.script(id))
    }

    /// Returns or inserts a `ScriptGitStaging` entry for `script_id`.
    pub fn ensure_script_git_staging(&mut self, script_id: u32) -> &mut ScriptGitStaging {
        self.cold.script_git_staging.entry(script_id).or_default()
    }

    /// Builds a `Script` value from the workspace entry for `script_id`.
    pub fn script_to_ingest(&self, script_id: u32) -> Script {
        let s = self.script(script_id);
        Script {
            key: s.key(),
            kind: s.kind(),
            abs_path: s.abs_path(),
            checksum: s.checksum().copied(),
        }
    }
}
