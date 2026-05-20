mod baseline;
mod struct_sizes;

pub use baseline::{FootprintBaseline, StructSizeEntry, BASELINE_VERSION, THRESHOLD_BYTES};
pub use struct_sizes::{collect_struct_sizes, struct_sizes_json, write_struct_sizes_json};
