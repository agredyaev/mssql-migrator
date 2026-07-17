#![allow(missing_docs)]

use serde::{Deserialize, Serialize};

pub const BASELINE_VERSION: u32 = 1;
pub const THRESHOLD_BYTES: usize = 40;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct StructSizeEntry {
    pub package: String,
    #[serde(rename = "type")]
    pub type_name: String,
    pub bytes: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FootprintBaseline {
    pub version: u32,
    pub rustc_version: String,
    pub target: String,
    pub threshold_bytes: usize,
    pub struct_sizes: Vec<StructSizeEntry>,
}

impl FootprintBaseline {
    pub fn current() -> Self {
        Self {
            version: BASELINE_VERSION,
            // Stamped by build.rs at compile time: runtime env vars are unset
            // under `cargo test`, which is how "unknown" baselines happened.
            rustc_version: env!("RMIG_RUSTC_VERSION").to_string(),
            target: env!("RMIG_BUILD_TARGET").to_string(),
            threshold_bytes: THRESHOLD_BYTES,
            struct_sizes: super::struct_sizes::collect_struct_sizes(),
        }
    }
}
