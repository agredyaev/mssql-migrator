use migrator_core::domain::Workspace;
use migrator_core::scan::scan_root;

#[test]
fn scan_rejects_backslash_path_component_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    let path = base.path().join("bad\\db/smoke/tables/t1.sql");
    std::fs::create_dir_all(path.parent().expect("parent")).expect("mkdir");
    std::fs::write(&path, "CREATE TABLE smoke.t1(id INT NOT NULL);\n").expect("write sql");

    let mut ws = Workspace::default();
    let err = scan_root(&mut ws, base.path().to_str().expect("utf8 path"))
        .expect_err("backslash inside a path component must not be normalized as a separator");

    assert!(
        err.to_string().contains("invalid character"),
        "unexpected error: {err}"
    );
    assert_eq!(ws.object_count(), 0);
}

#[test]
fn scan_rejects_backslash_transition_path_component_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    let object = base.path().join("dactests/smoke/tables/t1.sql");
    let transition = base
        .path()
        .join("dactests/bad\\schema/tables/_migrations/t1/001_deadbee_add.sql");
    std::fs::create_dir_all(object.parent().expect("object parent")).expect("mkdir object");
    std::fs::create_dir_all(transition.parent().expect("transition parent"))
        .expect("mkdir transition");
    std::fs::write(&object, "CREATE TABLE smoke.t1(id INT NOT NULL);\n").expect("write object");
    std::fs::write(&transition, "ALTER TABLE smoke.t1 ADD name INT NULL;\n")
        .expect("write transition");

    let mut ws = Workspace::default();
    let err = scan_root(&mut ws, base.path().to_str().expect("utf8 path"))
        .expect_err("transition path components must be validated before metadata capture");

    assert!(
        err.to_string().contains("invalid character"),
        "unexpected error: {err}"
    );
}

#[test]
fn scan_rejects_duplicate_transition_ordinal() {
    let base = tempfile::tempdir().expect("tempdir");
    let mig = base.path().join("dactests/smoke/tables/_migrations/t1");
    std::fs::create_dir_all(&mig).expect("mkdir migrations");
    // Two distinct migration files (different sha+slug) claim the same ordinal 001
    // for the same table. Distinct filenames so this triggers on any filesystem.
    std::fs::write(
        mig.join("001_deadbee_add.sql"),
        "ALTER TABLE smoke.t1 ADD a INT NULL;\n",
    )
    .expect("write first transition");
    std::fs::write(
        mig.join("001_c0ffee0_more.sql"),
        "ALTER TABLE smoke.t1 ADD b INT NULL;\n",
    )
    .expect("write second transition");

    let mut ws = Workspace::default();
    let err = scan_root(&mut ws, base.path().to_str().expect("utf8 path"))
        .expect_err("two transitions with the same ordinal must be rejected");

    assert!(
        err.to_string().contains("duplicate transition ordinal"),
        "unexpected error: {err}"
    );
}

#[cfg(unix)]
#[test]
fn scan_skips_symlinked_sql_files() {
    use std::os::unix::fs::symlink;

    let base = tempfile::tempdir().expect("tempdir");
    let root = base.path().join("root");
    let external = base.path().join("external.sql");
    let real = root.join("dactests/smoke/tables/real.sql");
    let link = root.join("dactests/smoke/tables/link.sql");
    std::fs::create_dir_all(real.parent().expect("parent")).expect("mkdir");
    std::fs::write(&real, "CREATE TABLE smoke.real(id INT NOT NULL);\n").expect("write real");
    std::fs::write(&external, "CREATE TABLE smoke.link(id INT NOT NULL);\n").expect("write ext");
    symlink(&external, &link).expect("symlink");

    let mut ws = Workspace::default();
    scan_root(&mut ws, root.to_str().expect("utf8 path")).expect("scan");

    assert_eq!(ws.object_count(), 1);
    assert_eq!(ws.script_rows.len(), 1);
    let paths: Vec<_> = ws
        .scripts_iter()
        .map(|s| s.path_str().to_string())
        .collect();
    assert_eq!(paths, vec!["dactests/smoke/tables/real.sql"]);
}

fn scan_ok(base: &std::path::Path) -> Workspace {
    let mut ws = Workspace::default();
    scan_root(&mut ws, base.to_str().expect("utf8 path")).expect("scan");
    ws
}

fn write_sql(base: &std::path::Path, rel: &str, body: &str) {
    let p = base.join(rel);
    std::fs::create_dir_all(p.parent().expect("parent")).expect("mkdir");
    std::fs::write(p, body).expect("write sql");
}

/// Nested (archive/fixture) paths under a kind folder must fail loudly instead
/// of becoming live managed objects.
#[test]
fn scan_rejects_nested_object_path_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    write_sql(
        base.path(),
        "db/archive/dbo/tables/old.sql",
        "CREATE TABLE dbo.old(id INT);\n",
    );
    let mut ws = Workspace::default();
    let err = scan_root(&mut ws, base.path().to_str().expect("utf8"))
        .expect_err("nested object path must be rejected");
    assert!(
        err.to_string().contains("nested path"),
        "unexpected error: {err}"
    );
}

/// `.SQL` (any case) strips from the object key exactly like `.sql`.
#[test]
fn scan_uppercase_extension_keys_match_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    write_sql(
        base.path(),
        "db/smoke/tables/t.SQL",
        "CREATE TABLE smoke.t(id INT);\n",
    );
    let ws = scan_ok(base.path());
    assert_eq!(ws.object_count(), 1);
    assert_eq!(ws.entry_key(0).as_str(), "smoke/tables/t");
}

/// Schemas named `checks` or `_migrations` are ordinary schemas when they are
/// not at the reserved contract positions.
#[test]
fn scan_checks_and_migrations_schema_names_are_objects_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    write_sql(
        base.path(),
        "db/checks/views/v.sql",
        "CREATE OR ALTER VIEW checks.v AS SELECT 1 AS one;\n",
    );
    write_sql(
        base.path(),
        "db/_migrations/views/w.sql",
        "CREATE OR ALTER VIEW _migrations.w AS SELECT 1 AS one;\n",
    );
    let ws = scan_ok(base.path());
    assert_eq!(
        ws.object_count(),
        2,
        "both files are deployable view objects"
    );
}

/// The same schema name in two catalog databases stays two schema entries, so
/// each database subset can still plan CREATE SCHEMA.
#[test]
fn scan_shared_schema_name_exists_in_both_databases_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    write_sql(
        base.path(),
        "db_a/shared/tables/a.sql",
        "CREATE TABLE shared.a(id INT);\n",
    );
    write_sql(
        base.path(),
        "db_b/shared/tables/b.sql",
        "CREATE TABLE shared.b(id INT);\n",
    );
    let ws = scan_ok(base.path());
    let a = ws.for_catalog_database("db_a");
    let b = ws.for_catalog_database("db_b");
    assert_eq!(a.schemas.len(), 1, "db_a keeps its shared schema");
    assert_eq!(b.schemas.len(), 1, "db_b keeps its shared schema");
}

/// Equal transition ordinals for same-named tables in DIFFERENT databases are
/// legitimate; each database subset keeps only its own scripts.
#[test]
fn scan_same_ordinal_across_databases_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    for db in ["db_a", "db_b"] {
        write_sql(
            base.path(),
            &format!("{db}/dbo/tables/t.sql"),
            "CREATE TABLE dbo.t(id INT);\n",
        );
        write_sql(
            base.path(),
            &format!("{db}/dbo/tables/_migrations/t/001_abcdef1_add.sql"),
            "ALTER TABLE dbo.t ADD c INT;\n",
        );
    }
    let ws = scan_ok(base.path());
    for db in ["db_a", "db_b"] {
        let sub = ws.for_catalog_database(db);
        let total: usize = sub
            .object_entries
            .iter()
            .map(|object| object.transitions.len())
            .sum();
        assert_eq!(total, 1, "{db} keeps exactly its own transition");
    }
}

/// Control characters in repository paths must fail before they reach logs or
/// serialized plan output.
#[cfg(unix)]
#[test]
fn scan_rejects_control_char_names_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    write_sql(
        base.path(),
        "db/smoke/tables/a\nb.sql",
        "CREATE TABLE smoke.x(id INT);\n",
    );
    let mut ws = Workspace::default();
    let err = scan_root(&mut ws, base.path().to_str().expect("utf8"))
        .expect_err("control characters must be rejected");
    assert!(err.to_string().contains("invalid character"), "{err}");
}

#[test]
fn scan_accepts_valid_unicode_names_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    write_sql(
        base.path(),
        "db/smoke/tables/café.sql",
        "CREATE TABLE smoke.café(id INT);\n",
    );
    let ws = scan_ok(base.path());
    assert_eq!(ws.entry_key(0).as_str(), "smoke/tables/café");
}

#[test]
fn scan_rejects_oversized_sql_before_parsing() {
    let base = tempfile::tempdir().expect("tempdir");
    let path = base.path().join("db/smoke/tables/huge.sql");
    std::fs::create_dir_all(path.parent().expect("parent")).expect("mkdir");
    std::fs::write(&path, vec![b'x'; 4 * 1024 * 1024 + 1]).expect("write");
    let mut ws = Workspace::default();
    let err = scan_root(&mut ws, base.path().to_str().expect("utf8"))
        .expect_err("oversized SQL must fail");
    assert!(err.to_string().contains("byte limit"), "{err}");
}

/// Check scripts live directly under `checks/` at either contract position;
/// both are skipped silently without becoming deployable objects.
#[test]
fn scan_checks_under_schema_position_is_skipped_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    write_sql(
        base.path(),
        "db/smoke/checks/smoke_has_rows.sql",
        "SELECT COUNT(*) FROM smoke.t;\n",
    );
    write_sql(
        base.path(),
        "db/smoke/tables/t.sql",
        "CREATE TABLE smoke.t(id INT);\n",
    );
    let ws = scan_ok(base.path());
    assert_eq!(
        ws.object_count(),
        1,
        "check script must not become an object"
    );
    assert_eq!(ws.entry_key(0).as_str(), "smoke/tables/t");
}

/// Back-to-back path hashing let ["a.sql","b.sql"] collide with the single
/// path "a.sqlb.sql"; length-prefixing must keep the digests distinct.
#[test]
fn layout_digest_length_prefix_prevents_concat_collision_regression() {
    use migrator_core::domain::{Script, ScriptKey, ScriptKind};
    use migrator_core::scan::layout_digest;

    let mut two = Workspace::default();
    for p in ["db/s/views/a.sql", "db/s/views/b.sql"] {
        two.insert_script(Script {
            key: ScriptKey::from_path(p),
            kind: ScriptKind::Object,
            abs_path: p.into(),
            checksum: Some([0; 32]),
        });
    }

    let mut one = Workspace::default();
    let joined = "db/s/views/a.sqldb/s/views/b.sql";
    one.insert_script(Script {
        key: ScriptKey::from_path(joined),
        kind: ScriptKind::Object,
        abs_path: joined.into(),
        checksum: Some([0; 32]),
    });

    assert_ne!(layout_digest(&two), layout_digest(&one));
}
