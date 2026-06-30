//! Performance footprint helpers: struct-size reports, baseline snapshots, and layout audits.

mod baseline;
mod layout_report;
mod struct_sizes;

pub use baseline::{FootprintBaseline, StructSizeEntry, BASELINE_VERSION, THRESHOLD_BYTES};
pub use layout_report::layout_report_lines;
pub use struct_sizes::{collect_struct_sizes, struct_sizes_json, write_struct_sizes_json};
