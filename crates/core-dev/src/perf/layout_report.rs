#![allow(missing_docs)]

use std::collections::HashMap;
use std::mem::{align_of, offset_of, size_of};

use migrator_core::config::Config;
use migrator_core::db::state::CatalogObject;
use migrator_core::domain::{
    ObjectEntry, ObjectKey, ObjectRow, ParentRef, SchemaEntry, ScriptKey, ScriptRow, SharedStr,
    Workspace, WorkspaceCold,
};
use migrator_core::export::{MigrationPlan, PlannedGit, PlannedObject};

pub fn layout_report_lines() -> Vec<String> {
    let mut lines = vec![
        format!(
            "ObjectEntry: size={} align={}",
            size_of::<ObjectEntry>(),
            align_of::<ObjectEntry>()
        ),
        format!("  key_off @{}", offset_of!(ObjectEntry, key_off)),
        format!("  script_id @{}", offset_of!(ObjectEntry, script_id)),
        format!("  checksum @{}", offset_of!(ObjectEntry, checksum)),
        format!("  flags @{}", offset_of!(ObjectEntry, flags)),
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
            "Workspace: size={} align={} (hot shell; cold in Box)",
            size_of::<Workspace>(),
            align_of::<Workspace>()
        ),
        format!(
            "WorkspaceCold: size={} align={} (one Box/plan; heap = maps + arena + vecs)",
            size_of::<WorkspaceCold>(),
            align_of::<WorkspaceCold>()
        ),
        format!(
            "  HashMap headers x10 ≈{} B (empty)",
            10 * size_of::<HashMap<u32, u32>>()
        ),
        format!("  Vec headers x9 ≈{} B", 9 * size_of::<Vec<u8>>()),
        format!(
            "  root String + digest + metrics ≈{} B",
            size_of::<String>() + 32 + 2 * size_of::<usize>()
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
