use super::Workspace;

impl Workspace {
    /// Clears layout data before a fresh scan.
    pub fn reset_layout(&mut self) {
        self.object_entries.clear();
        self.ingest_key_index.clear();
        self.key_index.clear();
        self.script_rows.clear();
        self.script_checksums.clear();
        self.script_git.clear();
        self.script_key_index.clear();
        self.transitions_staging.clear();
        self.schemas.clear();
        self.database_names.truncate(1);
        self.catalog_flags = 0;
    }
}
