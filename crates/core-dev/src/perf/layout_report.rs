#![allow(missing_docs)]

use std::mem::{align_of, offset_of, size_of};

use migrator_core::config::Config;
use migrator_core::db::state::CatalogObject;
use migrator_core::domain::{
    ObjectEntry, ObjectKey, ParentRef, SchemaEntry, ScriptKey, ScriptRow, Workspace,
};
use migrator_core::export::{MigrationPlan, PlannedGit, PlannedObject};

pub fn layout_report_lines() -> Vec<String> {
    let mut lines = vec![
        format!(
            "ObjectEntry: size={} align={}",
            size_of::<ObjectEntry>(),
            align_of::<ObjectEntry>()
        ),
        format!("  key @{}", offset_of!(ObjectEntry, key)),
        format!("  script_id @{}", offset_of!(ObjectEntry, script_id)),
        format!("  checksum @{}", offset_of!(ObjectEntry, checksum)),
        format!("  db_exists @{}", offset_of!(ObjectEntry, db_exists)),
        format!("  db_id @{}", offset_of!(ObjectEntry, db_id)),
        format!(
            "ParentRef: size={} CatalogObject: size={} SchemaEntry: size={}",
            size_of::<ParentRef>(),
            size_of::<CatalogObject>(),
            size_of::<SchemaEntry>()
        ),
        format!(
            "String: size={} ObjectKey: size={} ScriptKey: size={}",
            size_of::<String>(),
            size_of::<ObjectKey>(),
            size_of::<ScriptKey>()
        ),
        format!(
            "Workspace: size={} align={} (owned object and script state)",
            size_of::<Workspace>(),
            align_of::<Workspace>()
        ),
        format!(
            "PlannedObject: size={} PlannedGit: size={} ScriptRow: size={} Config: size={}",
            size_of::<PlannedObject>(),
            size_of::<PlannedGit>(),
            size_of::<ScriptRow>(),
            size_of::<Config>()
        ),
        format!("MigrationPlan: size={}", size_of::<MigrationPlan>()),
    ];
    lines.sort();
    lines
}
