mod parse;

#[derive(Clone, Debug)]
pub struct ParsedColumn {
    pub name: String,
    pub type_name: String,
    pub nullable: bool,
}

pub fn parse_table_columns(sql: &str) -> Vec<ParsedColumn> {
    let body = parse::extract_column_body(sql);
    if body.is_empty() {
        return Vec::new();
    }
    parse::split_columns(&body)
        .iter()
        .filter_map(|p| parse::parse_column_def(p))
        .collect()
}

pub fn new_columns(
    file_cols: &[ParsedColumn],
    db_names: &std::collections::HashMap<String, bool>,
) -> Vec<ParsedColumn> {
    file_cols
        .iter()
        .filter(|c| !db_names.contains_key(&c.name))
        .cloned()
        .collect()
}

pub fn has_dropped_columns(
    file_cols: &[ParsedColumn],
    db_names: &std::collections::HashMap<String, bool>,
) -> bool {
    let file_set: std::collections::HashSet<_> =
        file_cols.iter().map(|c| c.name.as_str()).collect();
    db_names.keys().any(|n| !file_set.contains(n.as_str()))
}
