use std::collections::HashMap;

use migrator_core::config::Config;
use migrator_core::db::TableColumn;
use migrator_core::domain::{
    share, Action, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, Workspace,
};
use migrator_core::export::{MigrationPlan, PlannedObject};
use migrator_core::scaffold;

#[test]
fn blocked_table_creates_scaffold_file() {
    let base = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.sql_base = base.path().to_string_lossy().into();
    cfg.database = "dactests".into();
    let plan = MigrationPlan {
        blocked: true,
        objects: vec![PlannedObject {
            normalized_key: "r/tables/t1".into(),
            object_path: "r/tables/t1.sql".into(),
            schema_name: "r".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            planned_action: Action::ReprocessChangedBlocked,
            exists: true,
            checksum: [0; 32],
            database_name: share("dactests"),
            parent_name: Default::default(),
            git: None,
            transition_paths: Vec::new(),
        }],
        ..Default::default()
    };
    let mut cols = HashMap::new();
    cols.insert(
        "r/tables/t1".into(),
        vec![
            TableColumn {
                name: "id".into(),
                type_name: "int".into(),
                nullable: false,
            },
            TableColumn {
                name: "name".into(),
                type_name: "nvarchar".into(),
                nullable: false,
            },
        ],
    );
    let created = scaffold::ensure(&cfg, &Workspace::default(), &plan, &cols).unwrap();
    assert!(created);
    let dir = base.path().join("dactests/r/tables/_migrations/t1");
    let content = std::fs::read_dir(&dir)
        .unwrap()
        .flatten()
        .find(|e| e.path().extension().map(|x| x == "sql").unwrap_or(false))
        .map(|e| std::fs::read_to_string(e.path()).unwrap())
        .unwrap();
    assert!(content.contains("-- rmig: transition-scaffold"));
    assert!(content.contains("[r].[t1]"));
}

#[test]
fn auto_add_column_when_file_has_new_col() {
    let base = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.sql_base = base.path().to_string_lossy().into();
    cfg.database = "dactests".into();
    let sql_path = base.path().join("r/tables/t1.sql");
    std::fs::create_dir_all(sql_path.parent().unwrap()).unwrap();
    std::fs::write(
        &sql_path,
        "CREATE TABLE [r].[t1] (\n  [id] INT NOT NULL,\n  [new_col] NVARCHAR(100) NULL\n)",
    )
    .unwrap();
    let sk = ScriptKey::from_path("r/tables/t1.sql");
    let mut ws = Workspace::default();
    let script_id = ws.insert_script(Script {
        key: sk.clone(),
        kind: ScriptKind::Object,
        abs_path: migrator_core::domain::share(sql_path.to_string_lossy().as_ref()),
        checksum: None,
    });
    let db_id = ws.intern_database(share("dactests"));
    ws.adopt_dense_entries(vec![ObjectEntry::with_staging_key(
        ObjectKey::new("r", "tables", "t1"),
        script_id,
        [0; 32],
        false,
        db_id,
    )]);
    let plan = MigrationPlan {
        blocked: true,
        objects: vec![PlannedObject {
            normalized_key: "r/tables/t1".into(),
            object_path: "r/tables/t1.sql".into(),
            schema_name: "r".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            planned_action: Action::ReprocessChangedBlocked,
            exists: true,
            checksum: [0; 32],
            database_name: share("dactests"),
            parent_name: Default::default(),
            git: None,
            transition_paths: Vec::new(),
        }],
        ..Default::default()
    };
    let mut cols = HashMap::new();
    cols.insert(
        "r/tables/t1".into(),
        vec![TableColumn {
            name: "id".into(),
            type_name: "int".into(),
            nullable: false,
        }],
    );
    assert!(scaffold::ensure(&cfg, &ws, &plan, &cols).unwrap());
    let dir = base.path().join("dactests/r/tables/_migrations/t1");
    let content = std::fs::read_dir(&dir)
        .unwrap()
        .flatten()
        .find(|e| e.path().extension().map(|x| x == "sql").unwrap_or(false))
        .map(|e| std::fs::read_to_string(e.path()).unwrap())
        .unwrap();
    assert!(!content.contains("-- rmig: transition-scaffold"));
    assert!(content.contains("ALTER TABLE"));
    assert!(content.contains("new_col"));
}

#[test]
fn auto_add_column_declines_when_table_drops_existing_column_edge_case() {
    let content = scaffold_for_table(
        "CREATE TABLE [r].[t1] (\n  [new_col] INT NULL\n)",
        vec![TableColumn {
            name: "id".into(),
            type_name: "int".into(),
            nullable: false,
        }],
    );

    assert!(content.contains("-- rmig: transition-scaffold"));
    assert!(!content.contains("ALTER TABLE"));
}

#[test]
fn auto_add_column_declines_unsafe_computed_column_regression() {
    let content = scaffold_for_table(
        "CREATE TABLE [r].[t1] (\n  [id] INT NOT NULL,\n  [calc] AS ([id] + 1)\n)",
        vec![TableColumn {
            name: "id".into(),
            type_name: "int".into(),
            nullable: false,
        }],
    );

    assert!(content.contains("-- rmig: transition-scaffold"));
    assert!(!content.contains("ALTER TABLE"));
}

#[test]
fn auto_add_column_declines_unbracketed_computed_column_regression() {
    let content = scaffold_for_table(
        "CREATE TABLE [r].[t1] (\n  [id] INT NOT NULL,\n  calc AS id + 1\n)",
        vec![TableColumn {
            name: "id".into(),
            type_name: "int".into(),
            nullable: false,
        }],
    );

    assert!(content.contains("-- rmig: transition-scaffold"));
    assert!(!content.contains("ALTER TABLE"));
}

fn scaffold_for_table(table_sql: &str, db_columns: Vec<TableColumn>) -> String {
    let base = tempfile::tempdir().unwrap();
    let mut cfg = Config::default();
    cfg.sql_base = base.path().to_string_lossy().into();
    cfg.database = "dactests".into();
    let sql_path = base.path().join("r/tables/t1.sql");
    std::fs::create_dir_all(sql_path.parent().unwrap()).unwrap();
    std::fs::write(&sql_path, table_sql).unwrap();
    let sk = ScriptKey::from_path("r/tables/t1.sql");
    let mut ws = Workspace::default();
    let script_id = ws.insert_script(Script {
        key: sk,
        kind: ScriptKind::Object,
        abs_path: migrator_core::domain::share(sql_path.to_string_lossy().as_ref()),
        checksum: None,
    });
    let db_id = ws.intern_database(share("dactests"));
    ws.adopt_dense_entries(vec![ObjectEntry::with_staging_key(
        ObjectKey::new("r", "tables", "t1"),
        script_id,
        [0; 32],
        false,
        db_id,
    )]);
    let plan = MigrationPlan {
        blocked: true,
        objects: vec![PlannedObject {
            normalized_key: "r/tables/t1".into(),
            object_path: "r/tables/t1.sql".into(),
            schema_name: "r".into(),
            kind: "tables".into(),
            object_name: "t1".into(),
            planned_action: Action::ReprocessChangedBlocked,
            exists: true,
            checksum: [0; 32],
            database_name: share("dactests"),
            parent_name: Default::default(),
            git: None,
            transition_paths: Vec::new(),
        }],
        ..Default::default()
    };
    let mut cols = HashMap::new();
    cols.insert("r/tables/t1".into(), db_columns);
    assert!(scaffold::ensure(&cfg, &ws, &plan, &cols).unwrap());
    let dir = base.path().join("dactests/r/tables/_migrations/t1");
    std::fs::read_dir(&dir)
        .unwrap()
        .flatten()
        .find(|e| e.path().extension().map(|x| x == "sql").unwrap_or(false))
        .map(|e| std::fs::read_to_string(e.path()).unwrap())
        .unwrap()
}
