use super::{load_toml_config, load_toml_config_required};

#[test]
fn missing_default_is_optional_but_explicit_path_is_required() {
    let path =
        std::env::temp_dir().join(format!("rmig-missing-config-{}.toml", std::process::id()));
    assert!(load_toml_config(&path).is_ok());
    let err = load_toml_config_required(&path).expect_err("explicit config must exist");
    assert!(err.to_string().contains("config file unreadable"), "{err}");
}

#[test]
fn rejects_toml_secrets_without_echoing_them() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("config.toml");
    for (body, variable) in [
        ("[database]\nuser = 'must-not-leak'\n", "RM_DB_USER"),
        ("[database]\npassword = 'must-not-leak'\n", "RM_DB_PASSWORD"),
        ("[session]\ntoken = 'must-not-leak'\n", "RMIG_SESSION_TOKEN"),
    ] {
        std::fs::write(&path, body).expect("write");
        let err = load_toml_config(&path).expect_err("secret must be rejected");
        assert!(err.to_string().contains(variable), "{err}");
        assert!(!err.to_string().contains("must-not-leak"), "{err}");
    }
}

#[test]
fn rejects_unknown_fields() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("config.toml");
    std::fs::write(&path, "[database]\nserveer = 'typo'\n").expect("write");
    let err = load_toml_config_required(&path).expect_err("unknown key must fail");
    assert!(err.to_string().contains("serveer"), "{err}");
}
