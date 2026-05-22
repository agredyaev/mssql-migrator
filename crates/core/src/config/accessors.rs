use super::flags::{
    flag_get, flag_set, CONFIG_FLAG_CATALOG_CACHE, CONFIG_FLAG_INSPECT_FULL, CONFIG_FLAG_JSON_LOGS,
    CONFIG_FLAG_REPORT_SYNC, CONFIG_FLAG_SKIP_GIT,
};
use super::Config;

impl Config {
    #[inline]
    pub fn report_sync(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_REPORT_SYNC)
    }

    #[inline]
    pub fn set_report_sync(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_REPORT_SYNC, on);
    }

    #[inline]
    pub fn skip_git(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_SKIP_GIT)
    }

    #[inline]
    pub fn set_skip_git(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_SKIP_GIT, on);
    }

    #[inline]
    pub fn json_logs(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_JSON_LOGS)
    }

    #[inline]
    pub fn set_json_logs(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_JSON_LOGS, on);
    }

    #[inline]
    pub fn inspect_full(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_INSPECT_FULL)
    }

    #[inline]
    pub fn set_inspect_full(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_INSPECT_FULL, on);
    }

    #[inline]
    pub fn catalog_cache(&self) -> bool {
        flag_get(self.flags, CONFIG_FLAG_CATALOG_CACHE)
    }

    #[inline]
    pub fn set_catalog_cache(&mut self, on: bool) {
        flag_set(&mut self.flags, CONFIG_FLAG_CATALOG_CACHE, on);
    }
}
