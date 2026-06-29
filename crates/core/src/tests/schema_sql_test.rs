use super::build_create_schema_sql;
use crate::error::EXIT_INVALID_INPUT;

#[test]
fn builds_idempotent_guarded_statement() {
    // The guard wraps CREATE SCHEMA in EXEC(...) because CREATE SCHEMA must be the
    // first statement in its batch; a plain `IF ... CREATE SCHEMA` is illegal T-SQL.
    let sql = build_create_schema_sql("reporting").expect("valid name");
    assert_eq!(
        sql,
        "IF SCHEMA_ID(N'reporting') IS NULL EXEC('CREATE SCHEMA [reporting]')"
    );
}

#[test]
fn escapes_single_quotes_in_both_literal_contexts() {
    // SCHEMA_ID probe -> O''Brien ; EXEC argument -> [O''Brien] (doubled again for
    // the outer EXEC string literal). Without this a quoted name would break out.
    let sql = build_create_schema_sql("O'Brien").expect("quotes are legal when bracketed");
    assert_eq!(
        sql,
        "IF SCHEMA_ID(N'O''Brien') IS NULL EXEC('CREATE SCHEMA [O''Brien]')"
    );
}

#[test]
fn escapes_closing_bracket() {
    let sql = build_create_schema_sql("a]b").expect("valid bracketed name");
    assert_eq!(
        sql,
        "IF SCHEMA_ID(N'a]b') IS NULL EXEC('CREATE SCHEMA [a]]b]')"
    );
}

#[test]
fn rejects_overlong_name() {
    let err = build_create_schema_sql(&"a".repeat(129)).expect_err("over 128 chars");
    assert_eq!(err.exit_code(), EXIT_INVALID_INPUT);
}

#[test]
fn rejects_path_separators_and_traversal() {
    for bad in ["..", ".", "a/b", "a\\b", ""] {
        assert!(
            build_create_schema_sql(bad).is_err(),
            "{bad:?} must be rejected before any SQL is built"
        );
    }
}
