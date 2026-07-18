//! Criterion 0.8 profiler backed by [`pprof`] flamegraphs.
//!
//! [`PprofGuard`] writes `ops/perf/artifacts/rust_<name>_flamegraph.svg` when `RMIG_PPROF=1`.
//! [`write_load_profile`] writes sustained-load flamegraph + text top frames.

mod criterion;
mod guard;
mod load;

pub use criterion::RmigPprofProfiler;
pub use guard::PprofGuard;
pub use load::{write_load_profile, write_text_top_frames};
