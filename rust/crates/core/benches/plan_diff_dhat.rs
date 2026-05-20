//! Heap allocation report for skip-heavy plan diff @ 5k (dhat).

mod bench_support;

use migrator_core::db::state::{CatalogState, ChecksumMap};
use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;

#[global_allocator]
static DHAT: dhat::Alloc = dhat::Alloc;

fn bench_setup() -> (
    migrator_core::domain::Workspace,
    CatalogState,
    ChecksumMap,
) {
    bench_support::skip_heavy_workspace(5000)
}

fn bench_warm(
    ws: &mut migrator_core::domain::Workspace,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
    plan: &mut MigrationPlan,
) {
    compute_diff_into(ws, catalog, checksums, plan).expect("warm");
}

fn bench_loop(
    ws: &mut migrator_core::domain::Workspace,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
    plan: &mut MigrationPlan,
    n: usize,
) {
    for _ in 0..n {
        compute_diff_into(ws, catalog, checksums, plan).expect("diff");
    }
}

fn main() {
    let _profiler = dhat::Profiler::new_heap();
    let (mut ws, catalog, checksums) = bench_setup();
    let mut plan = MigrationPlan::default();
    bench_warm(&mut ws, &catalog, &checksums, &mut plan);
    bench_loop(&mut ws, &catalog, &checksums, &mut plan, 20);
    eprintln!("dhat: plan_diff_skip_heavy_5000 x20 complete");
}
