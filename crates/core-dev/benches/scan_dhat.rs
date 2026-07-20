//! Heap allocation report for `scan_root` @ 5k (dhat).
//!
//! Profiler starts AFTER fixture build + warm scan, so the recorded heap is the
//! per-iteration `scan_root` cost only (parse + arena interning + git preload + digest).

use migrator_core::domain::Workspace;
use migrator_core::scan::scan_root;
use migrator_core_dev::bench::scan::{scan_fixture_workspace, temp_scan_root};

#[global_allocator]
static DHAT: dhat::Alloc = dhat::Alloc;

/// Named marker frame: the dhat classifier attributes stacks containing
/// `bench_loop` to the loop phase; without it every allocation reads as setup.
#[inline(never)]
fn bench_loop(root_str: &str) {
    for _ in 0..20 {
        let mut ws = Workspace::default();
        scan_root(&mut ws, root_str).expect("scan");
        std::hint::black_box(&ws);
    }
}

fn main() {
    let n = 5000;
    let root = temp_scan_root("dhat");
    let _warm = scan_fixture_workspace(&root, n); // build dir (unprofiled)
    let root_str = root.to_str().expect("utf8 root").to_string();
    {
        let mut ws = Workspace::default();
        scan_root(&mut ws, &root_str).expect("warm");
    }

    let _profiler = dhat::Profiler::new_heap();
    bench_loop(&root_str);
    eprintln!("dhat: scan_root n=5000 x20 complete (loop-only profile)");
}
