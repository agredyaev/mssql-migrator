//! Sustained plan diff load: warm fixture once, then tight-loop `compute_diff_into` under pprof.
//!
//! Env:
//! - `RMIG_PROFILE_SECS` - profile window (default 30)
//! - `RMIG_PPROF_FREQ` - sample rate Hz (default 1000)
//! - `RMIG_REPO_ROOT` - repo root for artifact paths (set by `ops/perf/footprint_bench.sh`)

use std::hint::black_box;
use std::path::PathBuf;
use std::time::{Duration, Instant};

use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;
use migrator_core_dev::bench::skip::skip_heavy_workspace;
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

    let (mut ws, catalog, checksums) = skip_heavy_workspace(5000);
    let mut plan = MigrationPlan::default();
    compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("warm");

    eprintln!("load profile: {secs}s @ {frequency}Hz, skip_heavy n=5000");
    let guard = pprof::ProfilerGuard::new(frequency).expect("pprof ProfilerGuard::new");
    let t0 = Instant::now();
    let deadline = t0 + Duration::from_secs(secs);
    let mut iterations: u64 = 0;
    while Instant::now() < deadline {
        let ms = compute_diff_into(
            black_box(&mut ws),
            black_box(&catalog),
            black_box(&checksums),
            black_box(&mut plan),
        )
        .expect("diff");
        black_box(ms);
        iterations += 1;
    }
    let elapsed = t0.elapsed();
    let iter_per_s = iterations as f64 / elapsed.as_secs_f64();
    eprintln!(
        "iterations: {iterations} ({iter_per_s:.0} iter/s, {:.1}s wall)",
        elapsed.as_secs_f64()
    );

    let root = std::env::var("RMIG_REPO_ROOT").unwrap_or_else(|_| ".".into());
    let artifacts = PathBuf::from(&root).join("ops/perf/artifacts");
    let svg = artifacts.join("plan_diff_5k_load_flamegraph.svg");
    let txt = artifacts.join("plan_diff_load_profile.txt");
    let meta = format!(
        "skip_heavy n=5000, {secs}s sustained load, {iterations} iters ({iter_per_s:.0} iter/s)"
    );
    write_load_profile(guard, &svg, &txt, &meta).expect("write load profile");
    eprintln!("CPU flamegraph: {}", svg.display());
    eprintln!("text summary:  {}", txt.display());
}
