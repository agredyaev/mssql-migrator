use std::collections::HashMap;
use std::fs::File;
use std::io::{self, Write};
use std::path::Path;

/// Write flamegraph SVG and a text file with top leaf frames from a sustained-load capture.
pub fn write_load_profile(
    guard: pprof::ProfilerGuard<'static>,
    svg_path: &Path,
    txt_path: &Path,
    meta: &str,
) -> io::Result<()> {
    let report = guard
        .report()
        .build()
        .map_err(|e| io::Error::other(format!("pprof report build: {e}")))?;

    if let Some(parent) = svg_path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    report
        .flamegraph(File::create(svg_path)?)
        .map_err(|e| io::Error::other(format!("flamegraph: {e}")))?;

    if let Some(parent) = txt_path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    write_text_top_frames(&report, txt_path, meta)
}

fn write_text_top_frames(report: &pprof::Report, path: &Path, meta: &str) -> io::Result<()> {
    let mut by_fn: HashMap<String, isize> = HashMap::new();
    let mut total: isize = 0;
    for (frames, count) in &report.data {
        total += count;
        // Credit each function ONCE per sampled stack: recursive or repeated
        // frames would otherwise push inclusive percentages past 100%.
        let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();
        for sym_stack in &frames.frames {
            if let Some(sym) = sym_stack.first() {
                let name = sym.name();
                let name = if name.is_empty() {
                    "<unknown>"
                } else {
                    name.as_str()
                };
                if seen.insert(name.to_string()) {
                    *by_fn.entry(name.to_string()).or_default() += count;
                }
            }
        }
    }

    let mut entries: Vec<_> = by_fn.into_iter().collect();
    entries.sort_by_key(|(_, c)| -(*c as i64));

    let mut out = File::create(path)?;
    writeln!(out, "# {meta}")?;
    writeln!(out, "# total samples: {total}")?;
    writeln!(
        out,
        "# top frames (inclusive; each stack frame credited once per sample)"
    )?;
    writeln!(out)?;
    for (name, count) in entries.into_iter().take(40) {
        let pct = if total > 0 {
            100.0 * count as f64 / total as f64
        } else {
            0.0
        };
        writeln!(out, "{count:>8}  {pct:>5.1}%  {name}")?;
    }
    Ok(())
}
