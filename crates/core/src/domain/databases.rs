use super::Workspace;

impl Workspace {
    /// Resolve catalog database segment by intern id. `0` = empty.
    pub fn database_name(&self, db_id: u16) -> &str {
        self.database_names
            .get(db_id as usize)
            .map(String::as_str)
            .unwrap_or("")
    }

    /// Number of interned database names (callers gate on this before interning
    /// user-controlled names; see `scan::parse`).
    pub fn database_count(&self) -> usize {
        self.database_names.len()
    }

    /// Registers `name` in the database name list and returns its position; returns `0` for an empty name.
    pub fn intern_database(&mut self, name: String) -> u16 {
        if name.is_empty() {
            return 0;
        }
        // SQL Server database identifiers are case-insensitive, so two top-level
        // directories differing only in case ("Sales"/"sales") target the same
        // database and must intern to the same id (first-seen casing wins),
        // letting the duplicate-object guard catch collisions across them.
        if let Some(i) = self
            .database_names
            .iter()
            .position(|database| database.eq_ignore_ascii_case(&name))
        {
            return i as u16;
        }
        let id = self.database_names.len();
        assert!(id < u16::MAX as usize, "too many distinct database names");
        self.database_names.push(name);
        id as u16
    }
}

#[cfg(test)]
mod tests {
    use super::Workspace;

    #[test]
    fn intern_database_folds_case_variants_to_one_id() {
        let mut ws = Workspace::default();
        let a = ws.intern_database("Sales".into());
        let b = ws.intern_database("sales".into());
        assert_eq!(a, b, "case-variant db directories must share one id");
        assert_ne!(a, ws.intern_database("Other".into()));
    }
}
