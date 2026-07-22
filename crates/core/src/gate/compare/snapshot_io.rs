use crate::gate::snapshot::PlanSnapshot;

/// Deserializes a `PlanSnapshot` from JSON.
pub fn read_snapshot_json(data: &str) -> Result<PlanSnapshot, serde_json::Error> {
    serde_json::from_str(data)
}

/// Writes `snap` to `path` as pretty-printed JSON, creating parent directories as needed.
pub fn write_snapshot_file(path: &std::path::Path, snap: &PlanSnapshot) -> std::io::Result<()> {
    let data = serde_json::to_string_pretty(snap).map_err(std::io::Error::other)?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    std::fs::write(path, format!("{data}\n"))
}
