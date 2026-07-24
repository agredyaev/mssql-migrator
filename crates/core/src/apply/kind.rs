//! Object-kind ordering and transaction-batch sorting.
//!
//! ### Purpose
//! `kind_rank` assigns a deterministic order to SQL object kinds so that
//! applies respect dependency order (types → sequences → tables → synonyms →
//! indexes → views → functions → procedures → triggers). `sort_tx_batch`
//! applies this ordering to a mutable slice of planned objects.

/// Deterministic rank for a SQL object kind (lower = applied first).
///
/// Order: types (0), sequences (1), tables (2), synonyms (3), indexes (4),
/// views (5), functions (6), procedures (7), triggers (8), unknown (99).
fn kind_rank(kind: &str) -> i32 {
    match kind {
        "types" => 0,
        "sequences" => 1,
        "tables" => 2,
        "synonyms" => 3,
        "indexes" => 4,
        "views" => 5,
        "functions" => 6,
        "procedures" => 7,
        "triggers" => 8,
        _ => 99,
    }
}

/// Sort a batch of planned objects by kind rank then normalized key.
pub fn sort_tx_batch(batch: &mut [&crate::export::PlannedObject]) {
    batch.sort_by_key(|o| (kind_rank(o.kind.as_str()), o.normalized_key.as_str()));
}
