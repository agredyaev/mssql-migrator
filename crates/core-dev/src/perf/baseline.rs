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
            rustc_version: rustc_version(),
            target: std::env::var("TARGET").unwrap_or_else(|_| "unknown".into()),
            threshold_bytes: THRESHOLD_BYTES,
            struct_sizes: super::struct_sizes::collect_struct_sizes(),
        }
    }
}

fn rustc_version() -> String {
    option_env!("RUSTC_VERSION")
        .unwrap_or("unknown")
        .to_string()
}
