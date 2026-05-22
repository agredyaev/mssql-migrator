use std::env;

pub(super) fn changed_paths_from_env() -> Option<Vec<String>> {
    env::var("RMIG_GATE_CHANGED_FILES")
        .ok()
        .and_then(|raw| parse_csv_paths(&raw))
        .or_else(|| {
            env::var("RMIG_CHANGED_FILES")
                .ok()
                .and_then(|raw| parse_csv_paths(&raw))
        })
}

fn parse_csv_paths(raw: &str) -> Option<Vec<String>> {
    let paths: Vec<_> = raw
        .split(',')
        .map(|s| s.trim().replace('\\', "/"))
        .filter(|s| !s.is_empty())
        .collect();
    if paths.is_empty() {
        None
    } else {
        Some(paths)
    }
}
