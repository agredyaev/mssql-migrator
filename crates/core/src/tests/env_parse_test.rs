use super::validate_boolean_value;

#[test]
fn invalid_boolean_is_rejected_regression() {
    for value in ["ture", "", "2", "true-ish"] {
        let err = validate_boolean_value("RM_DB_ENCRYPT", value).expect_err("must reject");
        assert!(err.to_string().contains("RM_DB_ENCRYPT"));
    }
    for value in ["true", "FALSE", "1", "0", "Enabled", "disabled"] {
        validate_boolean_value("RM_DB_ENCRYPT", value).expect("recognized boolean");
    }
}
