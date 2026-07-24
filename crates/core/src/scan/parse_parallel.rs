//! Parallel object parse (file read + sha256) with an order-preserving merge.
//!
//! [`super::parse_object::parse_object`] is pure per file, so reads run across
//! worker threads and merge into `Workspace` in the original sorted order.

use std::path::PathBuf;
use std::thread;

use crate::error::{Error, Result};

use super::parse_object::{parse_object, ParsedObject};

/// Below this object count, thread setup outweighs the parallel file reads.
const PARALLEL_THRESHOLD: usize = 64;
/// Cap workers: scan is I/O-bound, so more threads mostly add scheduling noise.
const MAX_WORKERS: usize = 8;

/// Parse each `(rel, abs)` object script, preserving input order.  Returns `None`
/// per entry for files `parse_object` skips (e.g. unsupported object-type folder).
pub fn parse_objects(items: &[(String, PathBuf)]) -> Result<Vec<Option<ParsedObject>>> {
    if items.len() < PARALLEL_THRESHOLD {
        return items
            .iter()
            .map(|(rel, abs)| parse_object(rel, abs))
            .collect();
    }
    let workers = thread::available_parallelism()
        .map(|n| n.get())
        .unwrap_or(1)
        .min(MAX_WORKERS);
    let chunk = items.len().div_ceil(workers);
    thread::scope(|scope| {
        let handles: Vec<_> = items
            .chunks(chunk)
            .map(|c| {
                scope.spawn(|| {
                    c.iter()
                        .map(|(rel, abs)| parse_object(rel, abs))
                        .collect::<Vec<_>>()
                })
            })
            .collect();
        let mut out = Vec::with_capacity(items.len());
        for handle in handles {
            let part = handle
                .join()
                .map_err(|_| Error::Other("scan parse worker panicked".into()))?;
            out.extend(part);
        }
        out.into_iter().collect()
    })
}
