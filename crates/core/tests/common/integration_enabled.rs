pub fn enabled() -> bool {
    let on = std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false);
    // e2e runner scripts export RMIG_REQUIRE_INTEGRATION (ops/perf/e2e_env.sh):
    // a runner-invoked suite must never "pass" by silently skipping every test.
    if !on && std::env::var_os("RMIG_REQUIRE_INTEGRATION").is_some() {
        panic!(
            "RMIG_REQUIRE_INTEGRATION is set but RMIG_RUN_SQLSERVER_INTEGRATION is not \
             enabled — e2e runner invoked without integration env (broken env plumbing?)"
        );
    }
    on
}
