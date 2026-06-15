use super::default_log_filter;

#[test]
fn default_log_filter_scopes_simple_level_to_project_crates() {
    assert_eq!(
        default_log_filter("debug"),
        "warn,migrator_core=debug,rmig=debug,rmigd=debug"
    );
}

#[test]
fn default_log_filter_preserves_explicit_env_filter() {
    assert_eq!(
        default_log_filter("migrator_core=trace,tiberius=warn"),
        "migrator_core=trace,tiberius=warn"
    );
}

#[test]
fn default_log_filter_empty_level_defaults_to_info() {
    assert_eq!(
        default_log_filter("  "),
        "warn,migrator_core=info,rmig=info,rmigd=info"
    );
}
