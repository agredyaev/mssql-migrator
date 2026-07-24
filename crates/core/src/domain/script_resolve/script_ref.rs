use crate::domain::{Script, ScriptGit, ScriptKind, ScriptRow, Workspace};

/// Borrowed script view.
pub struct ScriptRef<'a> {
    pub(super) ws: &'a Workspace,
    pub(super) id: u32,
}

impl<'a> ScriptRef<'a> {
    /// Returns the stored script row.
    pub fn row(&self) -> &'a ScriptRow {
        self.ws.script_row(self.id)
    }

    /// Clones the script key.
    pub fn key(&self) -> crate::domain::ScriptKey {
        self.row().key.clone()
    }

    /// Returns the repository-relative path.
    pub fn path_str(&self) -> &'a str {
        self.row().key.as_str()
    }

    /// Returns the absolute path.
    pub fn abs_path(&self) -> &'a str {
        &self.row().abs_path
    }

    /// Returns the script kind.
    pub fn kind(&self) -> ScriptKind {
        self.row().kind
    }

    /// Returns the script checksum.
    pub fn checksum(&self) -> Option<&'a [u8; 32]> {
        self.ws.script_checksum(self.id)
    }

    /// Returns the Git commit hash.
    pub fn git_hash(&self) -> &'a str {
        self.git_field(|git| git.hash.as_ref())
    }

    /// Returns the Git author.
    pub fn git_author(&self) -> &'a str {
        self.git_field(|git| git.author.as_ref())
    }

    /// Returns the Git commit date.
    pub fn git_date(&self) -> &'a str {
        self.git_field(|git| git.date.as_ref())
    }

    fn git_field(&self, field: impl Fn(&ScriptGit) -> Option<&String>) -> &'a str {
        self.ws
            .script_git
            .get(&self.id)
            .and_then(field)
            .map(String::as_str)
            .unwrap_or("")
    }
}

impl Workspace {
    /// Returns the stored script row for a 1-based id.
    pub fn script_row(&self, script_id: u32) -> &ScriptRow {
        &self.script_rows[script_id as usize - 1]
    }

    /// Returns a borrowed script view.
    pub fn script(&self, script_id: u32) -> ScriptRef<'_> {
        ScriptRef {
            ws: self,
            id: script_id,
        }
    }

    /// Returns the stored checksum.
    pub fn script_checksum(&self, script_id: u32) -> Option<&[u8; 32]> {
        self.script_checksums
            .get(script_id as usize - 1)
            .and_then(Option::as_ref)
    }

    /// Iterates over all scripts.
    pub fn scripts_iter(&self) -> impl Iterator<Item = ScriptRef<'_>> + '_ {
        (1..=self.script_rows.len() as u32).map(|id| self.script(id))
    }

    /// Returns or inserts Git metadata for a script.
    pub fn ensure_script_git(&mut self, script_id: u32) -> &mut ScriptGit {
        self.script_git.entry(script_id).or_default()
    }

    /// Copies a stored script into the scan-ingest shape.
    pub fn script_to_ingest(&self, script_id: u32) -> Script {
        let script = self.script(script_id);
        Script {
            key: script.key(),
            kind: script.kind(),
            abs_path: script.abs_path().to_owned(),
            checksum: script.checksum().copied(),
        }
    }
}
