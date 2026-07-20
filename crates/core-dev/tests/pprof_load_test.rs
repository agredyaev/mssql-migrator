use std::collections::HashMap;
use std::time::SystemTime;

use migrator_core_dev::pprof::write_text_top_frames;
use pprof::{Frames, Report, Symbol};

fn sym(name: &str) -> Symbol {
    Symbol {
        name: Some(name.as_bytes().to_vec()),
        addr: None,
        lineno: None,
        filename: None,
    }
}

/// A function appearing twice in one sampled stack (recursion) must be
/// credited once per sample, keeping inclusive counts <= total samples.
#[test]
fn repeated_frame_in_one_stack_credited_once_regression() {
    let frames = Frames {
        frames: vec![vec![sym("recur")], vec![sym("recur")], vec![sym("leaf")]],
        thread_name: "t".into(),
        thread_id: 0,
        sample_timestamp: SystemTime::now(),
    };
    let mut data = HashMap::new();
    data.insert(frames, 7isize);
    let report = Report {
        data,
        timing: Default::default(),
    };

    let dir = std::env::temp_dir().join(format!("pprof-load-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    let path = dir.join("top.txt");
    write_text_top_frames(&report, &path, "test").unwrap();
    let text = std::fs::read_to_string(&path).unwrap();
    let _ = std::fs::remove_dir_all(&dir);

    assert!(text.contains("# total samples: 7"), "{text}");
    assert!(
        text.lines()
            .any(|l| l.ends_with("recur") && l.trim_start().starts_with("7 ")),
        "recur must be credited exactly once (7 samples), got:\n{text}"
    );
    assert!(
        !text.contains("      14"),
        "double-credited recursive frame:\n{text}"
    );
}
