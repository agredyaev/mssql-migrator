use migrator_core::domain::{StringArenaBuilder, StringInterner};

#[test]
fn dedups_repeated_strings() {
    let mut interner = StringInterner::with_capacity(4);
    let a = interner.intern("schema");
    let b = interner.intern("schema");
    assert_eq!(a, b);
    assert_eq!(interner.unique_count(), 1);
}

#[test]
fn arena_single_buffer() {
    let mut b = StringArenaBuilder::with_capacity(32, 4);
    b.register("schema");
    b.register("views");
    b.register("schema");
    let arena = b.finish();
    assert_eq!(arena.unique_count(), 2);
    assert_eq!(arena.byte_len(), "schema".len() + "views".len());
    assert_eq!(arena.get("schema"), arena.get("schema"));
}
