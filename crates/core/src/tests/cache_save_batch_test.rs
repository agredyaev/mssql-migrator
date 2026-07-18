use super::CACHE_SAVE_BATCH;

/// Rows and layout metadata must publish atomically: every statement of the
/// cache save sits inside one explicit transaction with XACT_ABORT on.
#[test]
fn cache_save_batch_wraps_delete_insert_merge_in_one_transaction_regression() {
    assert!(CACHE_SAVE_BATCH.starts_with("SET XACT_ABORT ON;"));
    let begin = CACHE_SAVE_BATCH.find("BEGIN TRANSACTION").expect("BEGIN");
    let commit = CACHE_SAVE_BATCH
        .rfind("COMMIT TRANSACTION")
        .expect("COMMIT");
    for stmt in [
        "DELETE FROM azdo_deploy_meta.catalog_cache",
        "INSERT INTO azdo_deploy_meta.catalog_cache",
        "MERGE azdo_deploy_meta.catalog_meta",
    ] {
        let i = CACHE_SAVE_BATCH
            .find(stmt)
            .unwrap_or_else(|| panic!("missing statement: {stmt}"));
        assert!(begin < i && i < commit, "{stmt} outside the transaction");
    }
}
