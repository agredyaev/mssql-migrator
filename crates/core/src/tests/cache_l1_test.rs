use crate::cache::l1::L1Cache;
use crate::db::state::{CatalogState, ChecksumMap};

fn digest(byte: u8) -> [u8; 32] {
    [byte; 32]
}

#[test]
fn l1_save_load_roundtrip_happy_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    let cache = L1Cache::new(&dir.path().to_string_lossy());
    let mut checksums = ChecksumMap::new();
    checksums.insert_normalized("smoke/tables/t1", [7u8; 32]);
    let catalog = CatalogState::default();
    cache
        .save("srv~db", &digest(1), &checksums, &catalog)
        .expect("save");
    let hit = cache
        .try_load("srv~db", &digest(1))
        .expect("load")
        .expect("hit");
    assert_eq!(hit.0.get_normalized("smoke/tables/t1"), Some(&[7u8; 32]));
}

#[test]
fn l1_digest_mismatch_and_missing_are_misses_negative_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    let cache = L1Cache::new(&dir.path().to_string_lossy());
    cache
        .save(
            "fp",
            &digest(1),
            &ChecksumMap::new(),
            &CatalogState::default(),
        )
        .expect("save");
    assert!(cache.try_load("fp", &digest(2)).expect("load").is_none());
    assert!(cache.try_load("other", &digest(1)).expect("load").is_none());
}

#[test]
fn l1_corrupt_payload_is_a_miss_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    let cache = L1Cache::new(&dir.path().to_string_lossy());
    let fp_dir = dir.path().join("fp");
    std::fs::create_dir_all(&fp_dir).expect("mkdir");
    std::fs::write(
        fp_dir.join(format!("{}.json", hex::encode(digest(3)))),
        b"not json",
    )
    .expect("write");
    assert!(cache.try_load("fp", &digest(3)).expect("load").is_none());
}

#[test]
fn l1_invalidate_all_clears_fingerprint_edge_case() {
    let dir = tempfile::tempdir().expect("tempdir");
    let cache = L1Cache::new(&dir.path().to_string_lossy());
    cache
        .save(
            "fp",
            &digest(4),
            &ChecksumMap::new(),
            &CatalogState::default(),
        )
        .expect("save");
    cache.invalidate_all("fp").expect("invalidate");
    assert!(cache.try_load("fp", &digest(4)).expect("load").is_none());
}
