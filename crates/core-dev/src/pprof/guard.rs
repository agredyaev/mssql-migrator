#![allow(missing_docs)]

use std::fs::File;
use std::path::PathBuf;

/// Integration-test guard: `RMIG_PPROF=1` writes `ops/perf/artifacts/rust_<name>_flamegraph.svg` on drop.
pub struct PprofGuard {
    guard: Option<pprof::ProfilerGuard<'static>>,
    out: PathBuf,
}

impl PprofGuard {
    pub fn new(name: &str) -> Self {
        let enabled = std::env::var("RMIG_PPROF")
            .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
            .unwrap_or(false);
        let out = PathBuf::from("ops/perf/artifacts").join(format!("rust_{name}_flamegraph.svg"));
        let guard = if enabled {
            pprof::ProfilerGuard::new(100).ok()
        } else {
            None
        };
        Self { guard, out }
    }
}

impl Drop for PprofGuard {
    fn drop(&mut self) {
        let Some(guard) = self.guard.take() else {
            return;
        };
        if let Ok(report) = guard.report().build() {
            if let Some(parent) = self.out.parent() {
                let _ = std::fs::create_dir_all(parent);
            }
            if let Ok(file) = File::create(&self.out) {
                let _ = report.flamegraph(file);
                eprintln!("wrote flamegraph: {}", self.out.display());
            }
        }
    }
}
