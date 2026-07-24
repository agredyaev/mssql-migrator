use super::super::Workspace;

impl Workspace {
    /// Returns a workspace containing only objects and scripts for `database`.
    pub fn for_catalog_database(&self, database: &str) -> Workspace {
        let prefix = format!("{database}/");
        let mut out = Workspace {
            root: self.root.clone(),
            layout_digest: self.layout_digest,
            schemas: self
                .schemas
                .iter()
                .filter(|schema| schema.database == database)
                .cloned()
                .collect(),
            ..Workspace::default()
        };
        let db_id = out.intern_database(database.to_owned());
        let mut script_ids = std::collections::HashMap::new();
        for id in 1..=self.script_rows.len() as u32 {
            if self.script(id).path_str().starts_with(&prefix) {
                script_ids.insert(id, out.insert_script(self.script_to_ingest(id)));
            }
        }
        let pairs = self
            .object_entries
            .iter()
            .filter(|object| self.database_name(object.db_id) == database)
            .map(|object| {
                let mut copy = object.clone();
                copy.db_id = db_id;
                copy.parent = None;
                copy.transitions.clear();
                copy.script_id = script_ids
                    .get(&object.script_id)
                    .copied()
                    .unwrap_or_else(|| out.insert_script(self.script_to_ingest(object.script_id)));
                copy
            })
            .collect();
        out.adopt_dense_entries(pairs);
        out.relink_transitions_from(self, database);
        out.relink_parents_from(self, database);
        out
    }
}
