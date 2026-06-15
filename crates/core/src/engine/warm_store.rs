use crate::db::state::{CatalogState, ChecksumMap};

pub fn store_plan_db_snapshot(
    server_database: &str,
    digest: &[u8; 32],
    checksums: &ChecksumMap,
    catalog: &CatalogState,
) {
    crate::db::warm_snapshot::store(server_database, *digest, checksums.clone(), catalog.clone());
}

pub fn clear_plan_db_snapshot() {
    crate::db::warm_snapshot::clear();
}
