//! `RMIG_GATE_MAX_PLAN_WALL_MS` parsing is fail-closed: a present malformed
//! or non-positive value must never silently disable the SLO safeguard.

use migrator_core::gate::max_plan_wall_ms_from_env;

const VAR: &str = "RMIG_GATE_MAX_PLAN_WALL_MS";

// Env vars are process-global; keep every case in ONE test to avoid races
// with parallel test threads.
#[test]
fn slo_env_parsing_fail_closed_regression() {
    std::env::remove_var(VAR);
    assert_eq!(
        max_plan_wall_ms_from_env(),
        Ok(0),
        "absent disables the SLO"
    );

    std::env::set_var(VAR, "1500");
    assert_eq!(max_plan_wall_ms_from_env(), Ok(1500));

    for bad in ["4s", "-1", "0", "abc"] {
        std::env::set_var(VAR, bad);
        assert!(
            max_plan_wall_ms_from_env().is_err(),
            "{bad:?} must fail closed"
        );
    }
    std::env::remove_var(VAR);
}
