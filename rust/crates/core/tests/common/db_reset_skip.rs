pub fn skip_db_reset() -> bool {
    matches!(
        std::env::var("RMIG_GATE_SKIP_DB_RESET").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}
