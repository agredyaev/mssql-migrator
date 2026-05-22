//! Heap allocation report for skip-heavy plan diff @ 5k (dhat).

use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;
use migrator_core_dev::bench::skip::skip_heavy_workspace;

#[global_allocator]
static DHAT: dhat::Alloc = dhat::Alloc;

fn main() {
    let _profiler = dhat::Profiler::new_heap();
    let (mut ws, catalog, checksums) = skip_heavy_workspace(5000);
    let mut plan = MigrationPlan::default();
    compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("warm");
    for _ in 0..20 {
        compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("diff");
    }
    eprintln!("dhat: plan_diff_skip_heavy_5000 x20 complete");
}
