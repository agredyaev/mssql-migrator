use proptest::prelude::*;

use super::{parse_filename, parse_meta};

#[test]
fn parse_filename_happy_path() {
    let ord = parse_filename("001_abcdef1_add-col.sql").expect("valid");
    assert_eq!(ord, "001");
}

#[test]
fn parse_filename_rejects_contract_violations_negative_path() {
    for f in [
        "01_abcdef1_s.sql",
        "0001_abcdef1_s.sql",
        "001_abc_s.sql",
        "001_zzzzzzz_s.sql",
        "001_abcdef1_.sql",
        "001_abcdef1_s",
        "nonsense.sql",
        "",
    ] {
        assert!(parse_filename(f).is_none(), "should reject {f:?}");
    }
}

/// `history.normalized_key` is NVARCHAR(512): an overlong transition path
/// would be truncated on insert and replayed forever, so the scanner rejects it.
#[test]
fn parse_meta_rejects_overlong_path_regression() {
    let long = format!(
        "db/{}/tables/_migrations/{}/001_abcdef1_{}.sql",
        "s".repeat(120),
        "t".repeat(120),
        "x".repeat(260),
    );
    assert!(long.len() > 512, "fixture must exceed the column width");
    let err = match parse_meta(&long) {
        Err(e) => e,
        Ok(_) => panic!("overlong path must be rejected"),
    };
    assert!(
        err.to_string().contains("512"),
        "error names the limit: {err}"
    );
}

#[test]
fn parse_meta_requires_tables_migrations_shape_edge_case() {
    let ok = parse_meta("db/smoke/tables/_migrations/t1/001_abcdef1_s.sql").expect("no err");
    assert!(ok.is_some());
    for rel in [
        "db/smoke/views/_migrations/t1/001_abcdef1_s.sql",
        "db/smoke/tables/t1/001_abcdef1_s.sql",
        "tables/_migrations/t1/001_abcdef1_s.sql",
    ] {
        assert!(
            parse_meta(rel).expect("no err").is_none(),
            "should ignore {rel:?}"
        );
    }
}

proptest! {
    #[test]
    fn parse_filename_never_panics_fuzz(f in "\\PC*") {
        let _ = parse_filename(&f);
    }

    #[test]
    fn parse_meta_never_panics_on_sane_components_fuzz(
        parts in proptest::collection::vec("[a-zA-Z0-9._-]{1,12}", 0..9)
    ) {
        let rel = parts.join("/");
        let _ = parse_meta(&rel);
    }

    #[test]
    fn parse_filename_round_trip_fuzz(
        ord in 0u16..1000,
        commit in "[0-9a-f]{7,12}",
        slug in "[a-z0-9-]{1,20}",
    ) {
        let f = format!("{ord:03}_{commit}_{slug}.sql");
        let o = parse_filename(&f).expect("constructed name must parse");
        prop_assert_eq!(o, format!("{ord:03}"));
    }
}
