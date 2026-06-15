use std::sync::Arc;

use super::{empty_str, share, SharedStr, SharedStrInner};

fn arc_str_as_bytes(s: Arc<str>) -> Arc<[u8]> {
    // SAFETY: `str` and `[u8]` use the same slice metadata and allocation
    // layout, and every `str` is valid UTF-8 bytes for the lifetime of the Arc.
    unsafe { Arc::from_raw(Arc::into_raw(s) as *const [u8]) }
}

pub fn subslice_of(base: &SharedStr, part: &str) -> SharedStr {
    if part.is_empty() {
        return empty_str();
    }
    let full = base.as_str();
    let Some(off) = full.find(part) else {
        return share(part);
    };
    if &full[off..off + part.len()] != part {
        return share(part);
    }
    match &*base.0 {
        SharedStrInner::Slice { buf, start, .. } => SharedStr(Arc::new(SharedStrInner::Slice {
            buf: buf.clone(),
            start: start.saturating_add(off as u32),
            len: part.len() as u32,
        })),
        SharedStrInner::Owned(s) => SharedStr(Arc::new(SharedStrInner::Slice {
            buf: arc_str_as_bytes(s.clone()),
            start: off as u32,
            len: part.len() as u32,
        })),
        SharedStrInner::Empty => share(part),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;

    #[test]
    fn subslice_of_arena_slice_no_alloc() {
        let buf: Arc<[u8]> = Arc::from(b"schema/tables/t1.sql".as_slice());
        let base = SharedStr::from_arena_slice(buf, 0, 20);
        let part = SharedStr::subslice_of(&base, "tables");
        assert_eq!(part.as_str(), "tables");
        match &*part.0 {
            SharedStrInner::Slice { start, len, .. } => {
                assert_eq!(*start, 7);
                assert_eq!(*len, 6);
            }
            other => panic!("expected Slice, got {other:?}"),
        }
    }

    #[test]
    fn subslice_of_empty_part_returns_empty() {
        let base = share("schema/tables/t1");
        assert!(SharedStr::subslice_of(&base, "").is_empty());
    }

    #[test]
    fn subslice_of_owned_base_returns_slice_not_new_string() {
        let base = share("schema/tables/t1");
        let part = SharedStr::subslice_of(&base, "tables");
        assert_eq!(part.as_str(), "tables");
        match &*part.0 {
            SharedStrInner::Slice { start, len, .. } => {
                assert_eq!(*start, 7);
                assert_eq!(*len, 6);
            }
            other => panic!("expected Slice, got {other:?}"),
        }
    }

    #[test]
    fn subslice_of_owned_base_keeps_buffer_alive_after_base_drop() {
        let part = {
            let base = share("schema/tables/t1");
            SharedStr::subslice_of(&base, "tables")
        };

        assert_eq!(part.as_str(), "tables");
    }

    #[test]
    fn subslice_of_missing_part_falls_back_to_share() {
        let base = share("schema/tables/t1");
        let part = SharedStr::subslice_of(&base, "missing");
        assert_eq!(part.as_str(), "missing");
    }
}
