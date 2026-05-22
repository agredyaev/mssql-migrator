pub fn kind_rank(kind: &str) -> i32 {
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

pub fn sort_tx_batch(batch: &mut [&crate::export::PlannedObject]) {
    batch.sort_by_key(|o| (kind_rank(o.kind.as_ref()), o.normalized_key.as_ref()));
}
