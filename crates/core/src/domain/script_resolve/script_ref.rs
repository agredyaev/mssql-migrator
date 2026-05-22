use crate::domain::script::{ScriptGit, ScriptGitStaging, ScriptKind, ScriptRow};
use crate::domain::str_off::StrOff;
use crate::domain::{empty_str, SharedStr, Workspace};

/// Borrowed script view with derived field accessors.
pub struct ScriptRef<'a> {
    pub(super) ws: &'a Workspace,
    pub(super) id: u32,
}

impl<'a> ScriptRef<'a> {
    pub fn id(&self) -> u32 {
        self.id
    }

    pub fn row(&self) -> &'a ScriptRow {
        self.ws.script_row(self.id)
    }

    pub fn key(&self) -> crate::domain::ScriptKey {
        self.row().key(self.ws, self.id)
    }

    pub fn path_str(&self) -> &'a str {
        self.row().path_str(self.ws, self.id)
    }

    pub fn abs_path(&self) -> SharedStr {
        self.row().abs_path(self.ws, self.id)
    }

    pub fn kind(&self) -> ScriptKind {
        self.row().kind()
    }

    pub fn scaffold(&self) -> bool {
        self.row().scaffold()
    }

    pub fn checksum(&self) -> Option<&'a [u8; 32]> {
        if !self.row().has_checksum() {
            return None;
        }
        self.ws.script_checksum(self.id)
    }

    pub fn schema_part(&self) -> &'a str {
        super::paths::script_path_part(self.path_str(), 1)
    }

    pub fn object_kind_part(&self) -> &'a str {
        super::paths::script_path_part(self.path_str(), 2)
    }

    pub fn object_name_part(&self) -> &'a str {
        let path = self.path_str().trim_end_matches(".sql");
        path.rsplit('/').next().unwrap_or("")
    }

    pub fn git_hash(&self) -> SharedStr {
        self.git_field(|g| g.hash_off, |st| st.hash.as_ref())
    }

    pub fn git_author(&self) -> SharedStr {
        self.git_field(|g| g.author_off, |st| st.author.as_ref())
    }

    pub fn git_date(&self) -> SharedStr {
        self.git_field(|g| g.date_off, |st| st.date.as_ref())
    }

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
