use crate::sql_ident::{bracket_ident, validate_filename_token, validate_path_component};
use proptest::prelude::*;

fn safe_path_component() -> impl Strategy<Value = String> {
    prop::collection::vec(
        prop_oneof![
            Just('a'),
            Just('Z'),
            Just('0'),
            Just('_'),
            Just('-'),
            Just(']'),
            Just(' '),
        ],
        1..32,
    )
    .prop_map(|chars| chars.into_iter().collect::<String>())
    .prop_filter("must not collapse to reserved dot components", |s| {
        s != "." && s != ".."
    })
}

fn safe_filename_token() -> impl Strategy<Value = String> {
    prop::collection::vec(
        prop_oneof![Just('a'), Just('Z'), Just('0'), Just('_'), Just('-'),],
        1..64,
    )
    .prop_map(|chars| chars.into_iter().collect::<String>())
}

#[test]
fn quote_escapes_bracket() {
    assert_eq!(bracket_ident("a]b").unwrap(), "[a]]b]");
}

#[test]
fn rejects_dotdot() {
    assert!(validate_path_component("..").is_err());
}

#[test]
fn rejects_path_separators_and_nul_edge_cases() {
    for name in ["a/b", "a\\b", "a\0b", ""] {
        assert!(
            validate_path_component(name).is_err(),
            "expected invalid path component for {name:?}"
        );
    }
}

#[test]
fn filename_token_rejects_punctuation_negative_path() {
    assert!(validate_filename_token("sha:bad").is_err());
}

proptest! {
    #[test]
    fn validate_path_component_accepts_generated_safe_names_fuzz(name in safe_path_component()) {
        prop_assert!(validate_path_component(&name).is_ok());
    }

    #[test]
    fn bracket_ident_round_trips_generated_safe_names_fuzz(name in safe_path_component()) {
        let quoted = bracket_ident(&name).expect("safe name should quote");
        prop_assert!(quoted.starts_with('['));
        prop_assert!(quoted.ends_with(']'));
        let inner = &quoted[1..quoted.len() - 1];
        prop_assert_eq!(inner.replace("]]", "]"), name);
    }

    #[test]
    fn validate_filename_token_accepts_generated_safe_tokens_fuzz(token in safe_filename_token()) {
        prop_assert!(validate_filename_token(&token).is_ok());
    }
}

#[test]
fn bracket_ident_accepts_128_char_identifier() {
    let name = "a".repeat(128);
    assert!(
        bracket_ident(&name).is_ok(),
        "128 chars is the MSSQL maximum"
    );
}

#[test]
fn bracket_ident_rejects_overlong_identifier() {
    let name = "a".repeat(129);
    let err = bracket_ident(&name).expect_err("129 chars exceeds the MSSQL maximum");
    assert!(
        err.to_string().contains("identifier too long"),
        "unexpected error: {err}"
    );
}
