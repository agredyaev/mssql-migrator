pub(super) fn script_path_part(path: &str, index: usize) -> &str {
    let path = path.trim_end_matches(".sql");
    let mut parts = path.split('/');
    parts.nth(index).unwrap_or("")
}
