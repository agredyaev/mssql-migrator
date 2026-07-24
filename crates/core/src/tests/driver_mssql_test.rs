#[tokio::test]
async fn session_init_step_is_bounded_regression() {
    let err = super::with_connect_timeout(
        std::time::Duration::from_millis(1),
        "sql.example:1433",
        "session init",
        std::future::pending::<()>(),
    )
    .await
    .expect_err("stalled session init must time out");
    let msg = err.to_string();
    assert!(msg.contains("session init timed out"), "{msg}");
}
