//! Heap allocation report for the L1-cache serde round-trip @ 5k (dhat).
//!
//! Profiler starts AFTER the fixture build + warm round-trip, so the recorded heap
//! is the per-iteration serialize + deserialize cost only.

use migrator_core::db::state::CatalogState;
use migrator_core::db::ChecksumMap;
use migrator_core_dev::bench::skip::skip_heavy_workspace;

#[global_allocator]
static DHAT: dhat::Alloc = dhat::Alloc;

type Payload = (ChecksumMap, CatalogState);

fn main() {
    let n = 5000;
    let (_ws, catalog, checksums) = skip_heavy_workspace(n);
    let warm = serde_json::to_vec(&(&checksums, &catalog)).expect("warm serialize");
    let _warm: Payload = serde_json::from_slice(&warm).expect("warm deserialize");

    let _profiler = dhat::Profiler::new_heap();
    for _ in 0..20 {
        let bytes = serde_json::to_vec(&(&checksums, &catalog)).expect("serialize");
        let round: Payload = serde_json::from_slice(&bytes).expect("deserialize");
        std::hint::black_box(round);
    }
    eprintln!("dhat: cache_serde_roundtrip n=5000 x20 complete (loop-only profile)");
}
