use super::*;

fn argv(parts: &[&str]) -> Vec<String> {
    parts.iter().map(|part| (*part).to_string()).collect()
}

#[test]
fn parse_args_empty_invocation_returns_help_happy_path() {
    assert!(matches!(
        parse_args(&[]).expect("empty argv"),
        ParsedArgs::Help
    ));
}

#[test]
fn parse_args_accepts_flags_before_command_regression() {
    let parsed = parse_args(&argv(&["--json", "--env", "prod.env", "plan"]))
        .expect("flags before command should parse");
    match parsed {
        ParsedArgs::Run {
            cmd,
            env_file,
            json,
        } => {
            assert_eq!(cmd, "plan");
            assert_eq!(env_file.as_deref(), Some("prod.env"));
            assert!(json);
        }
        ParsedArgs::Help => panic!("expected run invocation"),
    }
}

#[test]
fn parse_args_accepts_flags_after_command_edge_case() {
    let parsed = parse_args(&argv(&["plan", "--json", "--env", "prod.env"]))
        .expect("flags after command should parse");
    match parsed {
        ParsedArgs::Run {
            cmd,
            env_file,
            json,
        } => {
            assert_eq!(cmd, "plan");
            assert_eq!(env_file.as_deref(), Some("prod.env"));
            assert!(json);
        }
        ParsedArgs::Help => panic!("expected run invocation"),
    }
}

#[test]
fn parse_args_missing_env_path_negative_path() {
    let err = match parse_args(&argv(&["plan", "--env"])) {
        Ok(_) => panic!("missing env path should fail"),
        Err(err) => err,
    };
    assert!(
        err.to_string().contains("--env requires a path"),
        "unexpected error: {err}"
    );
}

#[test]
fn parse_args_rejects_second_positional_arg_regression() {
    let err = match parse_args(&argv(&["plan", "extra"])) {
        Ok(_) => panic!("second positional arg should fail"),
        Err(err) => err,
    };
    assert!(
        err.to_string().contains("unexpected arg: extra"),
        "unexpected error: {err}"
    );
}

#[test]
fn parse_command_rejects_unknown_command_negative_path() {
    let err = parse_command("deploy").expect_err("unknown command should fail");
    assert!(
        err.to_string().contains("unknown command: deploy"),
        "unexpected error: {err}"
    );
}

/// `--env` must not swallow a following flag as its value: `--json` would be
/// silently lost and the missing path never diagnosed.
#[test]
fn parse_args_env_rejects_flag_as_value_regression() {
    let err = match parse_args(&argv(&["--env", "--json", "version"])) {
        Err(e) => e,
        Ok(_) => panic!("--env followed by a flag must be rejected"),
    };
    assert!(
        err.to_string().contains("--env requires a path"),
        "error: {err}"
    );

    let err = match parse_args(&argv(&["version", "--env"])) {
        Err(e) => e,
        Ok(_) => panic!("trailing --env without a value must be rejected"),
    };
    assert!(
        err.to_string().contains("--env requires a path"),
        "error: {err}"
    );
}
