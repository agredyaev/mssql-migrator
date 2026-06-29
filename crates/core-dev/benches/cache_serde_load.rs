//! Sustained L1-cache serde load: clone + serialize + deserialize the cached
//! `ChecksumMap`/`CatalogState` pair under pprof (mirrors `cache::l1::{save,try_load}`,
//! minus filesystem I/O — the pure-CPU serde cost).
//!
//! Env:
//! - `RMIG_PROFILE_SECS` - profile window (default 30)
//! - `RMIG_PPROF_FREQ` - sample rate Hz (default 1000)
//! - `RMIG_REPO_ROOT` - repo root for artifact paths (set by `ops/perf/footprint_bench.sh`)

use std::hint::black_box;
use std::path::PathBuf;
use std::time::{Duration, Instant};

use migrator_core::db::state::CatalogState;
use migrator_core::db::ChecksumMap;
use migrator_core_dev::bench::skip::skip_heavy_workspace;
use migrator_core_dev::pprof::write_load_profile;

type Payload = (ChecksumMap, CatalogState);

fn main() {
    let secs: u64 = std::env::var("RMIG_PROFILE_SECS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(30);
    let frequency: i32 = std::env::var("RMIG_PPROF_FREQ")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(1000);

    let n = 5000;
    let (_ws, catalog, checksums) = skip_heavy_workspace(n);
    let bytes = serde_json::to_vec(&(&checksums, &catalog)).expect("warm serialize");
    let _warm: Payload = serde_json::from_slice(&bytes).expect("warm deserialize");
    eprintln!(
        "cache serde load: {secs}s @ {frequency}Hz, skip_heavy n={n}, {} B payload",
        bytes.len()
    );

    let guard = pprof::ProfilerGuard::new(frequency).expect("pprof ProfilerGuard::new");
    let t0 = Instant::now();
    let deadline = t0 + Duration::from_secs(secs);
    let mut iterations: u64 = 0;
    while Instant::now() < deadline {
        let bytes = serde_json::to_vec(black_box(&(&checksums, &catalog))).expect("serialize");
        let round: Payload = serde_json::from_slice(black_box(&bytes)).expect("deserialize");
        black_box(round);
        iterations += 1;
    }
    let elapsed = t0.elapsed();
    let iter_per_s = iterations as f64 / elapsed.as_secs_f64();
    eprintln!(
        "iterations: {iterations} ({iter_per_s:.0} iter/s, {:.1}s wall)",
        elapsed.as_secs_f64()
    );

    let repo = std::env::var("RMIG_REPO_ROOT").unwrap_or_else(|_| ".".into());
    let artifacts = PathBuf::from(&repo).join("ops/perf/artifacts");
    let svg = artifacts.join("cache_serde_load_flamegraph.svg");
    let txt = artifacts.join("cache_serde_load_profile.txt");
    let meta = format!(
        "skip_heavy n={n}, {secs}s sustained load, {iterations} iters ({iter_per_s:.0} iter/s)"
    );
    write_load_profile(guard, &svg, &txt, &meta).expect("write load profile");
    eprintln!("CPU flamegraph: {}", svg.display());
    eprintln!("text summary:  {}", txt.display());
}
