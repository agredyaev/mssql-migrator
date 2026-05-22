use super::super::cold::WorkspaceCold;
use super::super::Workspace;
use crate::domain::share;

impl Workspace {
    pub fn for_catalog_database(&self, db: &str) -> Workspace {
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
        let pairs: Vec<_> = self
            .object_entries
            .iter()
            .enumerate()
            .filter(|(_, o)| self.database_name(o.db_id).as_ref() == db)
            .map(|(i, o)| {
                let mut e = o.clone();
                e.key_off = crate::domain::StrOff::EMPTY;
                e.db_id = db_id;
                (self.entry_key(i).clone(), e)
            })
            .collect();
        out.adopt_dense_entries(pairs);
        let prefix = format!("{db}/");
        for id in 1..=self.script_rows.len() as u32 {
            let s = self.script(id);
            if s.path_str().starts_with(prefix.as_str()) {
                out.insert_script(self.script_to_ingest(id));
            }
        }
        out.relink_transitions_from(self, db);
        out.relink_parents_from(self, db);
        if !out.object_entries.is_empty() {
            crate::domain::intern_workspace_strings(&mut out);
        }
        crate::domain::rebuild_path_caches(&mut out);
        out
    }
}
