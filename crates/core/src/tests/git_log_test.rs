use crate::git::log::parse_commit_line;

#[test]
fn parse_commit_line_happy_path() {
    let meta = parse_commit_line("COMMIT|abc1234|Alice|2026-01-02").expect("meta");
    assert_eq!(meta.hash, "abc1234");
    assert_eq!(meta.author, "Alice");
    assert_eq!(meta.date, "2026-01-02");
}

#[test]
fn parse_commit_line_rejects_malformed_negative_path() {
    for line in ["", "COMMIT|", "COMMIT||a|d", "nonsense", "COMMIT|hashonly"] {
        assert!(parse_commit_line(line).is_none(), "should reject {line:?}");
    }
}

#[test]
fn parse_commit_line_author_with_pipe_shifts_date_edge_case() {
    let meta = parse_commit_line("COMMIT|abc1234|A|B|2026-01-02").expect("meta");
    assert_eq!(meta.hash, "abc1234");
    assert_eq!(meta.author, "A");
    assert_eq!(meta.date, "B|2026-01-02");
}
