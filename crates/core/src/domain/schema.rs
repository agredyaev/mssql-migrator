use super::SharedStr;

/// Schema entry linking a database name to its normalized schema identifier.
#[derive(Clone, Debug)]
pub struct SchemaEntry {
    /// Database name this schema belongs to.
    pub database: SharedStr,
    /// Original schema name as declared in the repository.
    pub name: SharedStr,
    /// Lowercase-normalized schema name used for case-insensitive lookups.
    pub normalized: SharedStr,
}
