use std::collections::HashSet;

use super::{build_scope_json, InspectScope};

fn scope_with(keys: &[&str]) -> InspectScope {
    InspectScope {
        full_inspect: false,
        hot_keys: keys.iter().map(|s| s.to_string()).collect::<HashSet<_>>(),
        stable_objects: Default::default(),
        allow_l1_skip: false,
    }
}

#[test]
fn scope_json_is_sorted_for_stable_cache_keys() {
    // `scope_json` is the inspect-cache key; with a `HashSet` source its element
    // order would vary per run and defeat the cache. The output must be sorted.
    let scope = scope_with(&[
        "dbo/views/bar",
        "abc/procedures/baz",
        "dbo/tables/foo",
        "abc/tables/alpha",
    ]);
    let json = build_scope_json(&scope);
    let parsed: Vec<serde_json::Value> = serde_json::from_str(&json).expect("valid JSON array");
    let got: Vec<(String, String, String)> = parsed
        .iter()
        .map(|v| {
            (
                v["schema"].as_str().expect("schema").to_string(),
                v["kind"].as_str().expect("kind").to_string(),
                v["object"].as_str().expect("object").to_string(),
            )
        })
        .collect();
    let mut expected = got.clone();
    expected.sort();
    assert_eq!(got, expected, "scope_json must be emitted in sorted order");
    assert_eq!(got.len(), 4, "all keys must be present");
}

#[test]
fn scope_json_empty_is_bracket_pair() {
    assert_eq!(build_scope_json(&scope_with(&[])), "[]");
}
