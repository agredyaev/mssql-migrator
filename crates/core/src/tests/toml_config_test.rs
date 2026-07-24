use super::{load_toml_config, load_toml_config_required};
use crate::file_io::MAX_CONFIG_BYTES;

#[test]
fn missing_default_is_optional_but_explicit_path_is_required() {
    let path =
        std::env::temp_dir().join(format!("rmig-missing-config-{}.toml", std::process::id()));
    assert!(load_toml_config(&path).is_ok());
    let err = load_toml_config_required(&path).expect_err("explicit config must exist");
    assert!(err.to_string().contains("config file unreadable"), "{err}");
}

#[test]
fn rejects_environment_only_settings_without_echoing_them() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("config.toml");
    for (body, variable, value) in [
        (
            "[database]\nserver = 'must-not-leak'\n",
            "RM_DB_SERVER",
            "must-not-leak",
        ),
        ("[database]\nport = 1444\n", "RM_DB_PORT", "1444"),
        ("[database]\nencrypt = false\n", "RM_DB_ENCRYPT", "false"),
        (
            "[database]\ntrust_server_certificate = true\n",
            "RM_DB_TRUST_SERVER_CERTIFICATE",
            "true",
        ),
        (
            "[database]\nuser = 'must-not-leak'\n",
            "RM_DB_USER",
            "must-not-leak",
        ),
        (
            "[database]\npassword = 'must-not-leak'\n",
            "RM_DB_PASSWORD",
            "must-not-leak",
        ),
        (
            "[session]\nsocket = 'must-not-leak'\n",
            "RMIG_SESSION",
            "must-not-leak",
        ),
        (
            "[session]\ntoken = 'must-not-leak'\n",
            "RMIG_SESSION_TOKEN",
            "must-not-leak",
        ),
        (
            "[execution]\nallow_adopt = true\n",
            "RMIG_ALLOW_ADOPT",
            "true",
        ),
    ] {
        std::fs::write(&path, body).expect("write");
        let err = load_toml_config(&path).expect_err("environment-only setting must be rejected");
        assert!(err.to_string().contains(variable), "{err}");
        assert!(!err.to_string().contains(value), "{err}");
    }
}

#[test]
fn rejects_unknown_fields() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("config.toml");
    std::fs::write(&path, "[paths]\nsql_rooot = 'typo'\n").expect("write");
    let err = load_toml_config_required(&path).expect_err("unknown key must fail");
    assert!(err.to_string().contains("sql_rooot"), "{err}");
}

#[test]
fn malformed_toml_error_does_not_echo_source_line_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("config.toml");
    std::fs::write(&path, "[database]\npassword = \"must-not-leak\n").expect("write");
    let err = load_toml_config_required(&path).expect_err("malformed TOML must fail");
    assert!(!err.to_string().contains("must-not-leak"), "{err}");
    assert!(err.to_string().contains("invalid basic string"), "{err}");
}

#[test]
fn rejects_oversized_config_before_parsing() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("config.toml");
    std::fs::write(&path, vec![b' '; MAX_CONFIG_BYTES + 1]).expect("write");
    let err = load_toml_config_required(&path).expect_err("oversized config must fail");
    assert!(err.to_string().contains("byte limit"), "{err}");
}
