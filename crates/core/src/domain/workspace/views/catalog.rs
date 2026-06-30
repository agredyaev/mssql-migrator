use super::super::cold::WorkspaceCold;
use super::super::Workspace;
use crate::domain::share;
impl Workspace {
    /// Returns a new `Workspace` containing only the entries scoped to `db`.
    pub fn for_catalog_database(&self, db: &str) -> Workspace {
        let prefix = format!("{db}/");
        let mut out = Workspace {
            cold: Box::new(WorkspaceCold {
                root: self.root.clone(),
                layout_digest: self.layout_digest,
                schemas: self
                    .schemas
                    .iter()
                    .filter(|s| s.database.as_ref() == db)
                    .cloned()
                    .collect(),
                ..WorkspaceCold::new()
            }),
            ..Default::default()
        };
        // Child workspace starts with empty `database_names`; remap db_id (IDX) for this catalog.
        let db_id = out.intern_database(share(db));
        let mut child_script_ids = std::collections::HashMap::new();
        for id in 1..=self.script_rows.len() as u32 {
            let s = self.script(id);
            if s.path_str().starts_with(prefix.as_str()) {
                let child_id = out.insert_script(self.script_to_ingest(id));
                child_script_ids.insert(id, child_id);
            }
        }
        let mut pairs = Vec::new();
        for (i, o) in self.object_entries.iter().enumerate() {
            if self.database_name(o.db_id).as_ref() != db {
                continue;
            }
            let mut e = o.clone();
            e.key_off = crate::domain::StrOff::EMPTY;
            e.db_id = db_id;
            e.script_id = child_script_ids
                .get(&o.script_id)
                .copied()
                .unwrap_or_else(|| out.insert_script(self.script_to_ingest(o.script_id)));
            pairs.push((self.entry_key(i).clone(), e));
        }
        out.adopt_dense_entries(pairs);
        out.relink_transitions_from(self, db);
        out.relink_parents_from(self, db);
        if !out.object_entries.is_empty() {
            crate::domain::intern_workspace_strings(&mut out);
        }

        crate::domain::rebuild_path_caches(&mut out);

        out
    }
}
