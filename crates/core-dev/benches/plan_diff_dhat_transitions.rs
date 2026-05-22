//! dhat: table-heavy diff with transitions.

use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;
use migrator_core_dev::bench::table::table_heavy_workspace;

#[global_allocator]
static DHAT: dhat::Alloc = dhat::Alloc;

fn main() {
    let _profiler = dhat::Profiler::new_heap();
    let (mut ws, catalog, checksums) = table_heavy_workspace(500);
    let mut plan = MigrationPlan::default();
    compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("warm");
    for _ in 0..20 {
        compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("diff");
    }
    eprintln!("dhat: plan_diff_table_heavy_500 x20 complete");
}
