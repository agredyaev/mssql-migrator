//! dhat: real scan_root ingest @ 5k objects (CASE-1 scan tail measurement).

mod bench_support;

use migrator_core::db::state::{catalog_object_parts, CatalogState, ChecksumMap};
use migrator_core::domain::share;
use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;

#[global_allocator]
static DHAT: dhat::Alloc = dhat::Alloc;

fn bench_scan_setup() -> (
    migrator_core::domain::Workspace,
    CatalogState,
    ChecksumMap,
) {
    let root = bench_support::temp_scan_root("5k");
    let ws = bench_support::scan_fixture_workspace(&root, 5000);
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = ChecksumMap::new();
    for obj in &ws.object_entries {
        catalog.objects.insert(
            obj.key.clone(),
            catalog_object_parts(
                share("schema"),
                obj.kind.clone(),
                obj.name.clone(),
                None,
            ),
        );
        checksums.insert(obj.key.clone(), obj.checksum);
    }
    (ws, catalog, checksums)
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
    let (mut ws, catalog, checksums) = bench_scan_setup();
    let mut plan = MigrationPlan::default();
    bench_warm(&mut ws, &catalog, &checksums, &mut plan);
    bench_loop(&mut ws, &catalog, &checksums, &mut plan, 20);
    eprintln!("dhat: scan_fixture_5000 x20 complete");
}
