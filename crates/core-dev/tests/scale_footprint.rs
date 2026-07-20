//! Opt-in scale harness: measures the in-memory scan + plan-diff footprint at a
//! caller-chosen object count. Run under `/usr/bin/time -l` for authoritative
//! peak RSS. Gated on `RMIG_SCALE_N` so it never runs in an ordinary test pass.
//!
//!   RMIG_SCALE_N=100000 /usr/bin/time -l \
//!     cargo test -p migrator-core-dev --release --test scale_footprint -- --nocapture
//!
//! Prints self-sampled RSS (via `ps`) at each phase so a single run breaks the
//! footprint down by stage even without an external profiler.

use std::fmt::Write as _;
use std::time::Instant;

use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;
use migrator_core_dev::bench::scan::{scan_fixture_workspace, temp_scan_root};
use migrator_core_dev::bench::skip::skip_heavy_workspace;

fn rss_mb() -> f64 {
    let pid = std::process::id().to_string();
    let out = std::process::Command::new("ps")
        .args(["-o", "rss=", "-p", &pid])
        .output()
        .expect("ps");
    let kb: f64 = String::from_utf8_lossy(&out.stdout)
        .trim()
        .parse()
        .unwrap_or(0.0);
    kb / 1024.0
}

fn mark(label: &str) {
    let mut line = String::new();
    let _ = write!(line, "SCALE rss[{label}] = {:.1} MB", rss_mb());
    println!("{line}");
}

#[test]
fn scale_footprint_scan_and_diff() {
    let n: usize = match std::env::var("RMIG_SCALE_N")
        .ok()
        .and_then(|s| s.parse().ok())
    {
        Some(n) => n,
        None => {
            eprintln!("skip: set RMIG_SCALE_N to run the scale harness");
            return;
        }
    };
    println!("SCALE N = {n}");
    mark("start");

    // Phase 1: in-memory workspace + catalog + checksums (no disk, no DB).
    let t = Instant::now();
    let (mut ws, catalog, checksums) = skip_heavy_workspace(n);
    println!("SCALE build_ws_ms = {}", t.elapsed().as_millis());
    mark("after_build");

    // Phase 2: the plan diff over all N objects — the pipeline that holds the
    // whole workspace + catalog + plan at once (the unbounded-buffering risk).
    let mut plan = MigrationPlan::default();
    let t = Instant::now();
    compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("diff");
    println!("SCALE compute_diff_ms = {}", t.elapsed().as_millis());
    mark("after_diff");
    println!("SCALE plan_rows = {}", plan.rows.len());
    drop((ws, catalog, checksums, plan));
    mark("after_drop");

    // Phase 3: scan N .sql files off disk into a workspace (the scan_root path).
    let root = temp_scan_root("scale");
    let t = Instant::now();
    let scanned = scan_fixture_workspace(&root, n);
    println!("SCALE scan_files_ms = {}", t.elapsed().as_millis());
    println!("SCALE scanned_objects = {}", scanned.object_entries.len());
    mark("after_scan");
    let _ = std::fs::remove_dir_all(&root);
    assert_eq!(
        scanned.object_entries.len(),
        n,
        "scan must ingest every object"
    );
}
