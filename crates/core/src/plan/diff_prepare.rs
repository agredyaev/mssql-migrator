pub(crate) fn prior_digest_present(priors: &[Option<[u8; 32]>], row_index: usize) -> bool {
    priors
        .get(row_index)
        .and_then(|o| *o)
        .is_some_and(|cs| cs != [0; 32])
}
