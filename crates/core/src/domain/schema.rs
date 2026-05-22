use super::SharedStr;

#[derive(Clone, Debug)]
pub struct SchemaEntry {
    pub database: SharedStr,
    pub name: SharedStr,
    pub normalized: SharedStr,
}
