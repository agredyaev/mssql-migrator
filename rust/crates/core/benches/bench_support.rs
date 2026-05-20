use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};

use migrator_core::db::state::{catalog_object, CatalogState, ChecksumMap};
use migrator_core::domain::{
    empty_str, share, DbFacts, ObjectEntry, ObjectKey, SchemaEntry, Script, ScriptKey,
    StringInterner, Workspace,
};
use migrator_core::domain::ScriptKind;
use migrator_core::scan::scan_root;

pub fn skip_heavy_workspace(n: usize) -> (Workspace, CatalogState, ChecksumMap) {
    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = HashMap::with_capacity(n);
    let mut interner = StringInterner::with_capacity(n / 4 + 8);
    let schema_s = interner.intern("schema");
    let db_s = interner.intern("testdb");
    ws.schemas.push(SchemaEntry {
        database: share("testdb"),
        name: share("schema"),
        normalized: share("schema"),
    });
    let kinds = ["views", "procedures", "functions", "tables"];
    let kind_shared: Vec<_> = kinds.iter().map(|k| interner.intern(k)).collect();
    let mut entries = Vec::with_capacity(n);
    for i in 0..n {
        let kind_s = kind_shared[i % kinds.len()].clone();
        let name = interner.intern(&format!("obj_{i}"));
        let key = ObjectKey::from(interner.intern(&format!(
            "schema/{}/{}",
            kind_s.as_ref(),
            name.as_ref()
        )));
        let path = interner.intern(&format!(
            "testdb/schema/{}/{}.sql",
            kind_s.as_ref(),
            name.as_ref()
        ));
        let cs = checksum_for(i);
        let sk = ScriptKey::from(path.clone());
        ws.scripts.insert(
            sk.clone(),
            Script {
                key: sk.clone(),
                kind: ScriptKind::Object,
                abs_path: path.clone(),
                schema: schema_s.clone(),
                object_kind: kind_s.clone(),
                object_name: name.clone(),
                checksum: Some(cs),
                git_hash: empty_str(),
                git_author: empty_str(),
                git_date: empty_str(),
                table_name: None,
                scaffold: false,
            },
        );
        entries.push(ObjectEntry {
            key: key.clone(),
            script: sk,
            history: None,
            db: DbFacts {
                exists: true,
                parent: None,
            },
            plan: None,
            checksum: cs,
            schema: schema_s.clone(),
            kind: kind_s,
            name,
            database_name: db_s.clone(),
            parent_name: empty_str(),
            parent_key: None,
        });
        catalog.objects.insert(
            key.clone(),
            catalog_object("schema", kinds[i % kinds.len()], &format!("obj_{i}"), None),
        );
        checksums.insert(key, cs);
    }
    ws.string_arena_bytes = interner.byte_len();
    ws.string_arena_unique = interner.unique_count();
    ws.adopt_dense_entries(entries);
    migrator_core::plan::rebuild_transition_path_cache(&mut ws);
    (ws, catalog, checksums)
}

/// Tables with changed checksums and non-scaffold transition scripts (exercises paths_by_table).
pub fn table_heavy_workspace(
    n_tables: usize,
) -> (Workspace, CatalogState, ChecksumMap) {
    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = HashMap::with_capacity(n_tables);
    let mut interner = StringInterner::with_capacity(n_tables * 3 + 8);
    let schema_s = interner.intern("schema");
    let db_s = interner.intern("testdb");
    ws.schemas.push(SchemaEntry {
        database: share("testdb"),
        name: share("schema"),
        normalized: share("schema"),
    });
    let kind_s = interner.intern("tables");
    let mut entries = Vec::with_capacity(n_tables);
    for i in 0..n_tables {
        let name = interner.intern(&format!("t_{i}"));
        let key = ObjectKey::from(interner.intern(&format!(
            "schema/tables/{}",
            name.as_ref()
        )));
        let table_path = interner.intern(&format!(
            "testdb/schema/tables/{}.sql",
            name.as_ref()
        ));
        let file_cs = checksum_for(i + 1);
        let prior_cs = checksum_for(i);
        let sk = ScriptKey::from(table_path.clone());
        ws.scripts.insert(
            sk.clone(),
            Script {
                key: sk.clone(),
                kind: ScriptKind::Object,
                abs_path: table_path.clone(),
                schema: schema_s.clone(),
                object_kind: kind_s.clone(),
                object_name: name.clone(),
                checksum: Some(file_cs),
                git_hash: empty_str(),
                git_author: empty_str(),
                git_date: empty_str(),
                table_name: None,
                scaffold: false,
            },
        );
        entries.push(ObjectEntry {
            key: key.clone(),
            script: sk,
            history: None,
            db: DbFacts {
                exists: true,
                parent: None,
            },
            plan: None,
            checksum: file_cs,
            schema: schema_s.clone(),
            kind: kind_s.clone(),
            name: name.clone(),
            database_name: db_s.clone(),
            parent_name: empty_str(),
            parent_key: None,
        });
        for ord in ["001", "002"] {
            let trans_path = interner.intern(&format!(
                "testdb/schema/tables/{}/_migrations/{}_{}.sql",
                name.as_ref(),
                ord,
                name.as_ref()
            ));
            let tsk = ScriptKey::from(trans_path.clone());
            ws.scripts.insert(
                tsk.clone(),
                Script {
                    key: tsk.clone(),
                    kind: ScriptKind::Transition,
                    abs_path: trans_path.clone(),
                    schema: schema_s.clone(),
                    object_kind: share("transition"),
                    object_name: name.clone(),
                    checksum: Some([ord.as_bytes()[0]; 32]),
                    git_hash: empty_str(),
                    git_author: empty_str(),
                    git_date: empty_str(),
                    table_name: Some(name.as_ref().to_string()),
                    scaffold: false,
                },
            );
            ws.transitions_by_table
                .entry(key.clone())
                .or_default()
                .push((share(ord), tsk));
        }
        catalog.objects.insert(
            key.clone(),
            catalog_object("schema", "tables", &format!("t_{i}"), None),
        );
        checksums.insert(key, prior_cs);
    }
    ws.string_arena_bytes = interner.byte_len();
    ws.string_arena_unique = interner.unique_count();
    ws.adopt_dense_entries(entries);
    migrator_core::plan::rebuild_transition_path_cache(&mut ws);
    (ws, catalog, checksums)
}

/// Write `n` object SQL files and run `scan_root` (real scan ingest path).
pub fn scan_fixture_workspace(root: &Path, n: usize) -> Workspace {
    fs::create_dir_all(root.join("testdb/schema/views")).expect("mkdir views");
    for i in 0..n {
        let path = root.join(format!("testdb/schema/views/obj_{i}.sql"));
        fs::write(&path, format!("CREATE VIEW [schema].[obj_{i}] AS SELECT 1 AS x\n")).expect("write sql");
    }
    let mut ws = Workspace::default();
    scan_root(&mut ws, root.to_str().expect("utf8 root")).expect("scan_root");
    ws
}

pub fn temp_scan_root(prefix: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("rmig_scan_bench_{prefix}_{}", std::process::id()));
    let _ = fs::remove_dir_all(&dir);
    dir
}

fn checksum_for(i: usize) -> [u8; 32] {
    let b = (i % 256) as u8;
    [b; 32]
}
