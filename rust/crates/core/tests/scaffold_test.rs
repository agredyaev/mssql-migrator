use std::collections::HashMap;

use migrator_core::config::Config;
use migrator_core::db::TableColumn;
use migrator_core::domain::{
    Action, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, Workspace,
};
use migrator_core::export::{MigrationPlan, PlannedObject};
use migrator_core::scaffold;

#[test]
fn blocked_table_creates_scaffold_file() {
    let base = tempfile::tempdir().unwrap();
    let cfg = Config {
        sql_base: base.path().to_string_lossy().into(),
        database: "dactests".into(),
        ..Default::default()
    };
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
            database_name: Default::default(),
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
    let dir = base
        .path()
        .join("dactests/r/tables/_migrations/t1");
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
    let cfg = Config {
        sql_base: base.path().to_string_lossy().into(),
        database: "dactests".into(),
        ..Default::default()
    };
    let sql_path = base.path().join("r/tables/t1.sql");
    std::fs::create_dir_all(sql_path.parent().unwrap()).unwrap();
    std::fs::write(
        &sql_path,
        "CREATE TABLE [r].[t1] (\n  [id] INT NOT NULL,\n  [new_col] NVARCHAR(100) NULL\n)",
    )
    .unwrap();
    let sk = ScriptKey::from_path("r/tables/t1.sql");
    let mut ws = Workspace::default();
    ws.scripts.insert(
        sk.clone(),
        Script {
            key: sk.clone(),
            kind: ScriptKind::Object,
            abs_path: migrator_core::domain::share(sql_path.to_string_lossy().as_ref()),
            schema: "r".into(),
            object_kind: "tables".into(),
            object_name: "t1".into(),
            checksum: None,
            git_hash: migrator_core::domain::empty_str(),
            git_author: migrator_core::domain::empty_str(),
            git_date: migrator_core::domain::empty_str(),
            table_name: None,
            scaffold: false,
        },
    );
    ws.adopt_dense_entries(vec![ObjectEntry {
        key: ObjectKey::new("r", "tables", "t1"),
        script: sk,
        history: None,
        db: Default::default(),
        plan: None,
        checksum: [0; 32],
        schema: "r".into(),
        kind: "tables".into(),
        name: "t1".into(),
        database_name: Default::default(),
        parent_name: Default::default(),
        parent_key: None,
    }]);
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
            database_name: Default::default(),
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
