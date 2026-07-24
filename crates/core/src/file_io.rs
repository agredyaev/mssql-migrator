use std::io::{self, Read};
use std::path::Path;

pub(crate) const MAX_CONFIG_BYTES: usize = 1024 * 1024;
pub(crate) const MAX_SQL_SCRIPT_BYTES: usize = 4 * 1024 * 1024;

pub(crate) fn read_bounded(path: &Path, max_bytes: usize) -> io::Result<Vec<u8>> {
    let mut data = Vec::new();
    std::fs::File::open(path)?
        .take(max_bytes as u64 + 1)
        .read_to_end(&mut data)?;
    if data.len() > max_bytes {
        return Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!("file exceeds {max_bytes} byte limit"),
        ));
    }
    Ok(data)
}

#[cfg(test)]
mod tests {
    use super::read_bounded;

    #[test]
    fn accepts_exact_limit_and_rejects_one_byte_more() {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("input");
        std::fs::write(&path, [b'x'; 16]).expect("write exact");
        assert_eq!(read_bounded(&path, 16).expect("exact limit").len(), 16);
        std::fs::write(&path, [b'x'; 17]).expect("write oversized");
        assert_eq!(
            read_bounded(&path, 16)
                .expect_err("one byte over must fail")
                .kind(),
            std::io::ErrorKind::InvalidData
        );
    }
}
