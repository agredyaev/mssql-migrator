use std::mem::{align_of, offset_of, size_of};

use crate::config::Config;
use crate::db::state::CatalogObject;
use crate::domain::{
    ObjectEntry, ObjectKey, ObjectRow, ParentRef, SchemaEntry, ScriptRow, ScriptKey,
    SharedStr, Workspace,
};
use crate::export::{MigrationPlan, PlannedGit, PlannedObject};

pub fn layout_report_lines() -> Vec<String> {
    let mut lines = vec![
        format!(
            "ObjectEntry: size={} align={}",
            size_of::<ObjectEntry>(),
            align_of::<ObjectEntry>()
        ),
        format!("  key_off @{}", offset_of!(ObjectEntry, key_off)),
        format!(
            "  staging_key @{}",
            offset_of!(ObjectEntry, staging_key)
        ),
        format!("  script_id @{}", offset_of!(ObjectEntry, script_id)),
        format!("  checksum @{}", offset_of!(ObjectEntry, checksum)),
        format!("  db_exists @{}", offset_of!(ObjectEntry, db_exists)),
        format!("  db_id @{}", offset_of!(ObjectEntry, db_id)),
        format!(
            "ObjectRow: size={} align={}",
            size_of::<ObjectRow>(),
            align_of::<ObjectRow>()
        ),
        format!(
            "ParentRef: size={} CatalogObject: size={} SchemaEntry: size={}",
            size_of::<ParentRef>(),
            size_of::<CatalogObject>(),
            size_of::<SchemaEntry>()
        ),
        format!(
            "SharedStr: size={} ObjectKey: size={} ScriptKey: size={}",
            size_of::<SharedStr>(),
            size_of::<ObjectKey>(),
            size_of::<ScriptKey>()
        ),
        format!(
            "Workspace: size={} align={}",
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
