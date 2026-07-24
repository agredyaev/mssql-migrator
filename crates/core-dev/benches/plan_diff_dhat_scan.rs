//! dhat: real scan_root ingest @ 5k objects.

use migrator_core::db::state::{catalog_object_from_key, CatalogState, ChecksumMap};
use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;
use migrator_core_dev::bench::scan::{scan_fixture_workspace, temp_scan_root};

#[global_allocator]
static DHAT: dhat::Alloc = dhat::Alloc;

fn main() {
    let _profiler = dhat::Profiler::new_heap();
    let root = temp_scan_root("5k");
    let ws = scan_fixture_workspace(&root, 5000);
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = ChecksumMap::new();
    for obj in &ws.object_entries {
        let key = obj.key.clone();
        catalog
            .objects
            .insert(key.clone(), catalog_object_from_key(&key));
        checksums.insert_key(&key, obj.checksum);
    }
    let mut ws = ws;
    let mut plan = MigrationPlan::default();
    compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("warm");
    for _ in 0..20 {
        compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("diff");
    }
    eprintln!("dhat: scan_fixture_5000 x20 complete");
}
