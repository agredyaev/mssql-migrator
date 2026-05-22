#[inline]
pub(crate) fn flag_get(flags: u8, mask: u8) -> bool {
    flags & mask != 0
}

#[inline]
pub(crate) fn flag_set(flags: &mut u8, mask: u8, on: bool) {
    if on {
        *flags |= mask;
    } else {
        *flags &= !mask;
    }
}

pub const CONFIG_FLAG_REPORT_SYNC: u8 = 1 << 0;
pub const CONFIG_FLAG_SKIP_GIT: u8 = 1 << 1;
pub const CONFIG_FLAG_JSON_LOGS: u8 = 1 << 2;
pub const CONFIG_FLAG_INSPECT_FULL: u8 = 1 << 3;
pub const CONFIG_FLAG_CATALOG_CACHE: u8 = 1 << 4;

pub const CONFIG_COLD_FLAG_ENCRYPT: u8 = 1 << 0;
pub const CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE: u8 = 1 << 1;
