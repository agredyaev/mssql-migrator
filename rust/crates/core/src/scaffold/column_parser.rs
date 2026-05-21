#[derive(Clone, Debug)]
pub struct ParsedColumn {
    pub name: String,
    pub type_name: String,
    pub nullable: bool,
}

pub fn parse_table_columns(sql: &str) -> Vec<ParsedColumn> {
    let body = extract_column_body(sql);
    if body.is_empty() {
        return Vec::new();
    }
    split_columns(&body)
        .iter()
        .filter_map(|p| parse_column_def(p))
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

fn extract_column_body(sql: &str) -> String {
    let s = strip_line_comments(sql);
    let upper = s.to_uppercase().replace('\n', " ");
    let Some(idx) = upper.find('(') else {
        return String::new();
    };
    let body = &upper[idx + 1..];
    let Some(last) = body.rfind(')') else {
        return String::new();
    };
    body[..last].trim().to_string()
}

fn strip_line_comments(sql: &str) -> String {
    sql.lines()
        .map(|line| line.split_once("--").map(|(a, _)| a).unwrap_or(line))
        .collect::<Vec<_>>()
        .join("\n")
}

fn split_columns(body: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut depth = 0i32;
    let mut start = 0usize;
    for (i, ch) in body.char_indices() {
        match ch {
            '(' => depth += 1,
            ')' => depth -= 1,
            ',' if depth == 0 => {
                parts.push(body[start..i].trim().to_string());
                start = i + ch.len_utf8();
            }
            _ => {}
        }
    }
    if start < body.len() {
        parts.push(body[start..].trim().to_string());
    }
    parts
}

fn parse_column_def(def: &str) -> Option<ParsedColumn> {
    let def = def.trim();
    let upper = def.to_uppercase();
    if def.is_empty() || upper.starts_with("CONSTRAINT") || upper.starts_with("PRIMARY") {
        return None;
    }
    let (name, rest) = parse_name_rest(def)?;
    let (type_part, null_part) = split_type_null(&rest);
    let nullable = !null_part.to_uppercase().contains("NOT NULL");
    Some(ParsedColumn {
        name,
        type_name: type_part.trim().to_string(),
        nullable,
    })
}

fn parse_name_rest(def: &str) -> Option<(String, String)> {
    if let Some((n, r)) = def.split_once(']') {
        return Some((
            n.trim_start_matches('[').trim().to_lowercase(),
            r.trim().to_string(),
        ));
    }
    let mut parts = def.split_whitespace();
    let n = parts.next()?.to_lowercase();
    Some((n, parts.collect::<Vec<_>>().join(" ")))
}

fn split_type_null(rest: &str) -> (&str, &str) {
    let upper = rest.to_uppercase();
    if let Some(i) = upper.rfind("NOT NULL") {
        let (t, n) = rest.split_at(i);
        return (t.trim(), n.trim());
    }
    if let Some(i) = upper.rfind("NULL") {
        let (t, n) = rest.split_at(i);
        return (t.trim(), n.trim());
    }
    (rest, "")
}
