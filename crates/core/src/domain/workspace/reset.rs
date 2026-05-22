use super::Workspace;

impl Workspace {
    pub fn reset_layout(&mut self) {
        self.object_entries.clear();
        self.object_keys.clear();
        self.object_rows.clear();
        self.cold.ingest_key_index.clear();
        self.cold.ingest_keys.clear();
        self.cold.ingest_script_keys.clear();
        self.cold.ingest_script_abs.clear();
        self.cold.script_git_staging.clear();
        self.cold.key_index.clear();
        self.cold.fp_index.clear();
        self.script_rows.clear();
        self.script_checksums.clear();
        self.script_git.clear();
        self.script_key_index.clear();
        self.transitions_by_row.clear();
        self.transitions_staging.clear();
        self.parent_by_row.clear();
        self.parent_by_object.clear();
        self.invalidate_transition_paths();
        self.schemas.clear();
        self.string_arena_bytes = 0;
        self.string_arena_unique = 0;
        self.database_names.truncate(1);
        self.layout_arena = None;
        self.invalidate_catalog_facts();
    }
}
