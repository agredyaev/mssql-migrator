use std::path::PathBuf;

use migrator_core_dev::perf::{
    collect_struct_sizes, layout_report_lines, write_struct_sizes_json, FootprintBaseline,
};

fn baseline_path() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../core/tests/testdata/perf/footprint_baseline.json")
}

#[test]
fn struct_size_report() {
    for e in collect_struct_sizes() {
        eprintln!("{}::{}: {} bytes", e.package, e.type_name, e.bytes);
    }
    if std::env::var("RMIG_FOOTPRINT_REPORT").ok().as_deref() == Some("1") {
        for line in layout_report_lines() {
            eprintln!("layout: {line}");
        }
    }
    assert!(!collect_struct_sizes().is_empty());
    if std::env::var("RMIG_FOOTPRINT_REPORT").ok().as_deref() == Some("1") {
        let root = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../..");
        let out = root.join("ops/perf/artifacts/struct_sizes.json");
        write_struct_sizes_json(&out).expect("write struct_sizes.json");
        eprintln!("wrote {}", out.display());
    }
}

#[test]
fn footprint_baseline_match() {
    let path = baseline_path();
    let data = std::fs::read_to_string(&path).unwrap_or_else(|e| {
        panic!(
            "read baseline {}: {e} (run make bench-footprint-update-baseline)",
            path.display()
        )
    });
    let baseline: FootprintBaseline =
        serde_json::from_str(&data).expect("parse footprint baseline");
    // Struct sizes are ABI-specific: a baseline recorded for another target or
    // compiler must not be compared (legacy "unknown" baselines are exempt
    // until regenerated).
    let current = FootprintBaseline::current();
    if baseline.target != "unknown" {
        assert_eq!(
            baseline.target, current.target,
            "baseline recorded for a different target; regenerate it"
        );
    }
    let got = collect_struct_sizes();
    assert_eq!(
        got.len(),
        baseline.struct_sizes.len(),
        "struct count drift; update baseline if intentional"
    );
    for (i, (g, b)) in got.iter().zip(baseline.struct_sizes.iter()).enumerate() {
        assert_eq!(g, b, "struct_sizes[{i}]");
    }
}

#[test]
fn update_footprint_baseline() {
    if std::env::var("RMIG_FOOTPRINT_UPDATE_BASELINE")
        .ok()
        .as_deref()
        != Some("1")
    {
        return;
    }
    let path = baseline_path();
    let b = FootprintBaseline::current();
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).expect("mkdir baseline dir");
    }
    let data = serde_json::to_string_pretty(&b).expect("marshal");
    std::fs::write(&path, format!("{data}\n")).expect("write baseline");
    eprintln!("updated baseline at {}", path.display());
}

/// Provenance must be real: "unknown" baselines cannot be interpreted across
/// platforms.
#[test]
fn current_baseline_has_real_provenance_regression() {
    let cur = FootprintBaseline::current();
    assert_ne!(cur.target, "unknown", "build.rs must stamp TARGET");
    assert_ne!(cur.rustc_version, "unknown", "build.rs must stamp rustc");
    assert!(
        cur.target.contains('-'),
        "target triple expected: {}",
        cur.target
    );
}
