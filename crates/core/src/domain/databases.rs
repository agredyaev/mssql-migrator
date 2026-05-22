use super::shared::{empty_str, SharedStr};
use super::Workspace;

impl Workspace {
    /// Resolve catalog database segment by intern id. `0` = empty.
    pub fn database_name(&self, db_id: u16) -> SharedStr {
        self.database_names
            .get(db_id as usize)
            .cloned()
            .unwrap_or_else(empty_str)
    }

    pub fn intern_database(&mut self, name: SharedStr) -> u16 {
        if name.is_empty() {
            return 0;
        }
        if let Some(i) = self
            .database_names
            .iter()
            .position(|d| d.as_str() == name.as_str())
        {
            return i as u16;
        }
        let id = self.database_names.len();
        assert!(id < u16::MAX as usize, "too many distinct database names");
        self.database_names.push(name);
        id as u16
    }
}
