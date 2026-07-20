use crate::git::log::parse_commit_line;

const US: char = '\u{1f}';

fn line(hash: &str, author: &str, date: &str) -> String {
    format!("COMMIT{US}{hash}{US}{author}{US}{date}")
}

#[test]
fn parse_commit_line_happy_path() {
    let meta = parse_commit_line(&line("abc1234", "Alice", "2026-01-02")).expect("meta");
    assert_eq!(meta.hash, "abc1234");
    assert_eq!(meta.author, "Alice");
    assert_eq!(meta.date, "2026-01-02");
}

#[test]
fn parse_commit_line_rejects_malformed_negative_path() {
    for l in [
        String::new(),
        format!("COMMIT{US}"),
        format!("COMMIT{US}{US}a{US}d"),
        "nonsense".into(),
        // The old pipe-separated format is no longer a commit line.
        "COMMIT|abc1234|Alice|2026-01-02".into(),
        format!("COMMIT{US}hashonly"),
    ] {
        assert!(parse_commit_line(&l).is_none(), "should reject {l:?}");
    }
}

/// Git permits `|` in author names; the unit-separator format keeps author and
/// date fields intact instead of shifting the date into the author.
#[test]
fn parse_commit_line_author_with_pipe_keeps_fields_regression() {
    let meta = parse_commit_line(&line("abc1234", "A|B", "2026-01-02")).expect("meta");
    assert_eq!(meta.hash, "abc1234");
    assert_eq!(meta.author, "A|B");
    assert_eq!(meta.date, "2026-01-02");
}
