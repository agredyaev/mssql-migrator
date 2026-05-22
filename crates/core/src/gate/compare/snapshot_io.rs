use crate::gate::snapshot::PlanSnapshot;

pub fn read_snapshot_json(data: &str) -> Result<PlanSnapshot, serde_json::Error> {
    serde_json::from_str(data)
}

pub fn write_snapshot_json(snap: &PlanSnapshot) -> Result<String, serde_json::Error> {
    serde_json::to_string_pretty(snap)
}

pub fn write_snapshot_file(path: &std::path::Path, snap: &PlanSnapshot) -> std::io::Result<()> {
    let data = write_snapshot_json(snap).map_err(std::io::Error::other)?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    std::fs::write(path, format!("{data}\n"))
}
