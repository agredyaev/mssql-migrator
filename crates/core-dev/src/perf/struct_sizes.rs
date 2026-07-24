#![allow(missing_docs)]

use std::io;
use std::mem::size_of;
use std::path::Path;

use migrator_core::config::Config;
use migrator_core::domain::{ObjectEntry, ScriptRow, Workspace};
use migrator_core::export::{MigrationPlan, PlanSummary, PlannedObject};
use migrator_core::gate::{PlanSnapshot, SnapshotObject};
use migrator_core::timings::PhaseTimings;

use super::baseline::{StructSizeEntry, THRESHOLD_BYTES};

pub fn collect_struct_sizes() -> Vec<StructSizeEntry> {
    let raw = [
        entry("export", "MigrationPlan", size_of::<MigrationPlan>()),
        entry("export", "PlannedObject", size_of::<PlannedObject>()),
        entry("export", "PlanSummary", size_of::<PlanSummary>()),
        entry("domain", "Workspace", size_of::<Workspace>()),
        entry("domain", "ObjectEntry", size_of::<ObjectEntry>()),
        entry("domain", "ScriptRow", size_of::<ScriptRow>()),
        entry("gate", "PlanSnapshot", size_of::<PlanSnapshot>()),
        entry("gate", "SnapshotObject", size_of::<SnapshotObject>()),
        entry("config", "Config", size_of::<Config>()),
        entry("timings", "PhaseTimings", size_of::<PhaseTimings>()),
    ];
    let mut out: Vec<_> = raw
        .into_iter()
        .filter(|e| e.bytes >= THRESHOLD_BYTES)
        .collect();
    out.sort_by(|a, b| {
        b.bytes
            .cmp(&a.bytes)
            .then_with(|| a.package.cmp(&b.package))
            .then_with(|| a.type_name.cmp(&b.type_name))
    });
    out
}

pub fn struct_sizes_json() -> serde_json::Result<String> {
    let b = super::baseline::FootprintBaseline::current();
    serde_json::to_string_pretty(&b)
}

pub fn write_struct_sizes_json(path: &Path) -> io::Result<()> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    let json = struct_sizes_json().map_err(io::Error::other)?;
    std::fs::write(path, format!("{json}\n"))
}

fn entry(package: &str, type_name: &str, bytes: usize) -> StructSizeEntry {
    StructSizeEntry {
        package: package.into(),
        type_name: type_name.into(),
        bytes,
    }
}
