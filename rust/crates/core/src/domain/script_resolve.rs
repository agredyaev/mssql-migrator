use super::key::ScriptKey;
use super::script::{Script, ScriptGit, ScriptKind, ScriptRow};
use super::shared::{empty_str, SharedStr};
use super::str_off::StrOff;
use super::Workspace;

/// Borrowed script view (**DER** accessors).
pub struct ScriptRef<'a> {
    ws: &'a Workspace,
    id: u32,
}

impl<'a> ScriptRef<'a> {
    pub fn id(&self) -> u32 {
        self.id
    }

    pub fn row(&self) -> &'a ScriptRow {
        self.ws.script_row(self.id)
    }

    pub fn key(&self) -> ScriptKey {
        self.row().key(self.ws)
    }

    pub fn path_str(&self) -> &'a str {
        self.row().path_str(self.ws)
    }

    pub fn abs_path(&self) -> SharedStr {
        self.row().abs_path(self.ws)
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
        script_path_part(self.path_str(), 1)
    }

    pub fn object_kind_part(&self) -> &'a str {
        script_path_part(self.path_str(), 2)
    }

    pub fn object_name_part(&self) -> &'a str {
        let path = self.path_str().trim_end_matches(".sql");
        path.rsplit('/').next().unwrap_or("")
    }

    pub fn git_hash(&self) -> SharedStr {
        self.git_field(|g| g.hash_off, |g| g.staging_hash.as_ref())
    }

    pub fn git_author(&self) -> SharedStr {
        self.git_field(|g| g.author_off, |g| g.staging_author.as_ref())
    }

    pub fn git_date(&self) -> SharedStr {
        self.git_field(|g| g.date_off, |g| g.staging_date.as_ref())
    }

    pub fn has_git(&self) -> bool {
        !self.git_hash().is_empty() || !self.git_author().is_empty() || !self.git_date().is_empty()
    }

    fn git_field(
        &self,
        off: impl Fn(&ScriptGit) -> StrOff,
        staging: impl Fn(&ScriptGit) -> Option<&SharedStr>,
    ) -> SharedStr {
        let Some(git) = self.ws.script_git.get(&self.id) else {
            return empty_str();
        };
        if let Some(s) = staging(git) {
            return s.clone();
        }
        let o = off(git);
        if o.1 == 0 {
            return empty_str();
        }
        self.ws.shared_at(o)
    }
}

impl ScriptRow {
    pub fn key(&self, ws: &Workspace) -> ScriptKey {
        if let Some(k) = &self.staging_key {
            return k.clone();
        }
        ScriptKey::from(ws.shared_at(self.path_off))
    }

    pub fn path_str<'a>(&'a self, ws: &'a Workspace) -> &'a str {
        if let Some(k) = &self.staging_key {
            return k.as_str();
        }
        ws.str_at(self.path_off)
    }

    pub fn abs_path(&self, ws: &Workspace) -> SharedStr {
        if let Some(p) = &self.staging_abs_path {
            return p.clone();
        }
        ws.shared_at(self.abs_path_off)
    }
}

impl Workspace {
    pub fn script_row(&self, script_id: u32) -> &ScriptRow {
        &self.script_rows[(script_id - 1) as usize]
    }

    pub fn script_row_mut(&mut self, script_id: u32) -> &mut ScriptRow {
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

fn script_path_part(path: &str, index: usize) -> &str {
    let path = path.trim_end_matches(".sql");
    let mut parts = path.split('/');
    parts.nth(index).unwrap_or("")
}
