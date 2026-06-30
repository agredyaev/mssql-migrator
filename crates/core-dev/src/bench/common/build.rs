#![allow(missing_docs)]

use std::fmt::Write;

use migrator_core::domain::{StringArena, StringArenaBuilder};

/// Two-phase bench string build: register into one arena, no `StringInterner` / double finalize.
pub struct BenchBuild {
    builder: StringArenaBuilder,
}

impl BenchBuild {
    pub fn new(object_count: usize, extra_unique: usize) -> Self {
        Self {
            builder: StringArenaBuilder::with_capacity(
                object_count * 40,
                object_count * 3 + extra_unique,
            ),
        }
    }

    pub fn register(&mut self, s: &str) {
        self.builder.register(s);
    }

    pub fn register_static_schema(&mut self) {
        self.register("testdb");
        self.register("schema");
    }

    pub fn register_obj_name(&mut self, buf: &mut String, i: usize) {
        buf.clear();
        let _ = write!(buf, "obj_{i}");
        self.register(buf);
    }

    pub fn register_skip_heavy_row(
        &mut self,
        buf: &mut String,
        path: &mut String,
        i: usize,
        kind: &str,
    ) {
        self.register_obj_name(buf, i);
        buf.clear();
        let _ = write!(buf, "schema/{kind}/obj_{i}");
        self.register(buf);
        path.clear();
        let _ = write!(path, "testdb/schema/{kind}/obj_{i}.sql");
        self.register(path);
    }

    pub fn register_table_row(&mut self, buf: &mut String, path: &mut String, i: usize) {
        buf.clear();
        let _ = write!(buf, "t_{i}");
        self.register(buf);
        buf.clear();
        let _ = write!(buf, "schema/tables/t_{i}");
        self.register(buf);
        path.clear();
        let _ = write!(path, "testdb/schema/tables/t_{i}.sql");
        self.register(path);
    }

    pub fn register_table_transition(&mut self, path: &mut String, i: usize, ord: &str) {
        path.clear();
        let _ = write!(
            path,
            "testdb/schema/tables/t_{i}/_migrations/{ord}_t_{i}.sql"
        );
        self.register(path);
    }

    pub fn finish(self) -> StringArena {
        self.builder.finish()
    }
}
