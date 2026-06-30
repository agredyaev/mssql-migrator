use super::flags::{
    flag_get, flag_set, CONFIG_FLAG_CATALOG_CACHE, CONFIG_FLAG_INSPECT_FULL, CONFIG_FLAG_JSON_LOGS,
    CONFIG_FLAG_REPORT_SYNC, CONFIG_FLAG_SKIP_GIT,
};
use super::Config;

impl Config {
    #[inline]
    /// Returns `true` when the report-sync flag is set.
    pub fn report_sync(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_REPORT_SYNC)
    }

    #[inline]
    /// Sets or clears the report-sync flag.
    pub fn set_report_sync(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_REPORT_SYNC, on);
    }

    #[inline]
    /// Returns `true` when git operations should be skipped.
    pub fn skip_git(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_SKIP_GIT)
    }

    #[inline]
    /// Sets or clears the skip-git flag.
    pub fn set_skip_git(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_SKIP_GIT, on);
    }

    #[inline]
    /// Returns `true` when JSON-format logging is enabled.
    pub fn json_logs(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_JSON_LOGS)
    }

    #[inline]
    /// Sets or clears the JSON-log flag.
    pub fn set_json_logs(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_JSON_LOGS, on);
    }

    #[inline]
    /// Returns `true` when the full-inspect mode is enabled.
    pub fn inspect_full(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_INSPECT_FULL)
    }

    #[inline]
    /// Sets or clears the full-inspect flag.
    pub fn set_inspect_full(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_INSPECT_FULL, on);
    }

    #[inline]
    /// Returns `true` when the catalog cache is enabled.
    pub fn catalog_cache(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_CATALOG_CACHE)
    }

    #[inline]
    /// Sets or clears the catalog-cache flag.
    pub fn set_catalog_cache(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_CATALOG_CACHE, on);
    }
}
