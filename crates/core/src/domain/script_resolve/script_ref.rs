use crate::domain::script::{ScriptGit, ScriptGitStaging, ScriptKind, ScriptRow};
use crate::domain::str_off::StrOff;
use crate::domain::{empty_str, SharedStr, Workspace};

/// Borrowed script view with derived field accessors.
pub struct ScriptRef<'a> {
    pub(super) ws: &'a Workspace,
    pub(super) id: u32,
}

impl<'a> ScriptRef<'a> {
    /// Returns the numeric row index for this script.
    pub fn id(&self) -> u32 {
        self.id
    }

    /// Returns the underlying script metadata row.
    pub fn row(&self) -> &'a ScriptRow {
        self.ws.script_row(self.id)
    }

    /// Returns the script's composite key.
    pub fn key(&self) -> crate::domain::ScriptKey {
        self.row().key(self.ws, self.id)
    }

    /// Returns the repository-relative path string.
    pub fn path_str(&self) -> &'a str {
        self.row().path_str(self.ws, self.id)
    }

    /// Returns the absolute filesystem path.
    pub fn abs_path(&self) -> SharedStr {
        self.row().abs_path(self.ws, self.id)
    }

    /// Returns the script kind (SQL, scaffold, etc.).
    pub fn kind(&self) -> ScriptKind {
        self.row().kind()
    }

    /// Returns `true` if this script is a scaffold script.
    pub fn scaffold(&self) -> bool {
        self.row().scaffold()
    }

    /// Returns the SHA-256 checksum of the script content, if recorded.
    pub fn checksum(&self) -> Option<&'a [u8; 32]> {
        if !self.row().has_checksum() {
            return None;
        }
        self.ws.script_checksum(self.id)
    }

    /// Returns the schema segment of the script path.
    pub fn schema_part(&self) -> &'a str {
        super::paths::script_path_part(self.path_str(), 1)
    }

    /// Returns the object kind segment of the script path.
    pub fn object_kind_part(&self) -> &'a str {
        super::paths::script_path_part(self.path_str(), 2)
    }

    /// Returns the object name segment, stripping the `.sql` suffix.
    pub fn object_name_part(&self) -> &'a str {
        let path = self.path_str().trim_end_matches(".sql");
        path.rsplit('/').next().unwrap_or("")
    }

    /// Returns the Git commit hash associated with this script.
    pub fn git_hash(&self) -> SharedStr {
        self.git_field(|g| g.hash_off, |st| st.hash.as_ref())
    }

    /// Returns the Git author for the most recent commit to this script.
    pub fn git_author(&self) -> SharedStr {
        self.git_field(|g| g.author_off, |st| st.author.as_ref())
    }

    /// Returns the Git commit date for the most recent commit to this script.
    pub fn git_date(&self) -> SharedStr {
        self.git_field(|g| g.date_off, |st| st.date.as_ref())
    }

    /// Returns `true` if any Git metadata (hash, author, or date) is present.
    pub fn has_git(&self) -> bool {
        !self.git_hash().is_empty() || !self.git_author().is_empty() || !self.git_date().is_empty()
    }

    fn git_field(
        &self,
        off: impl Fn(&ScriptGit) -> StrOff,
        staging: impl Fn(&ScriptGitStaging) -> Option<&SharedStr>,
    ) -> SharedStr {
        if let Some(st) = self.ws.cold.script_git_staging.get(&self.id) {
            if let Some(s) = staging(st) {
                return s.clone();
            }
        }
        let Some(git) = self.ws.script_git.get(&self.id) else {
            return empty_str();
        };
        let o = off(git);
        if o.1 == 0 {
            return empty_str();
        }
        self.ws.shared_at(o)
    }
}
