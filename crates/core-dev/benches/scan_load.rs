//! Sustained scan load: build fixture tree once, then tight-loop `scan_root` under pprof.
//!
//! Env:
//! - `RMIG_PROFILE_SECS` - profile window (default 30)
//! - `RMIG_PPROF_FREQ` - sample rate Hz (default 1000)
//! - `RMIG_REPO_ROOT` - repo root for artifact paths (set by `ops/perf/footprint_bench.sh`)

use std::hint::black_box;
use std::path::PathBuf;
use std::time::{Duration, Instant};

use migrator_core::domain::Workspace;
use migrator_core::scan::scan_root;
use migrator_core_dev::bench::scan::{scan_fixture_workspace, temp_scan_root};
use migrator_core_dev::pprof::write_load_profile;

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
    let root = temp_scan_root("load");
    let _warm = scan_fixture_workspace(&root, n); // build dir + warm FS cache
    let root_str = root.to_str().expect("utf8 root").to_string();

    eprintln!("scan load profile: {secs}s @ {frequency}Hz, scan_fixture n={n}");
    let guard = pprof::ProfilerGuard::new(frequency).expect("pprof ProfilerGuard::new");
    let t0 = Instant::now();
    let deadline = t0 + Duration::from_secs(secs);
    let mut iterations: u64 = 0;
    while Instant::now() < deadline {
        let mut ws = Workspace::default();
        scan_root(black_box(&mut ws), black_box(&root_str)).expect("scan");
        black_box(&ws);
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
    let svg = artifacts.join("scan_5k_load_flamegraph.svg");
    let txt = artifacts.join("scan_load_profile.txt");
    let meta = format!(
        "scan_fixture n={n}, {secs}s sustained load, {iterations} iters ({iter_per_s:.0} iter/s)"
    );
    write_load_profile(guard, &svg, &txt, &meta).expect("write load profile");
    eprintln!("CPU flamegraph: {}", svg.display());
    eprintln!("text summary:  {}", txt.display());
}
