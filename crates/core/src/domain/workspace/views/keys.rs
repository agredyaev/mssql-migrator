use super::super::Workspace;

impl Workspace {
    /// Returns all normalized object keys (`schema/kind/name`) in workspace order.
    pub fn normalized_keys(&self) -> Vec<String> {
        self.object_entries
            .iter()
            .map(|object| object.key.shared())
            .collect()
    }

    /// JSON array of normalized keys for OPENJSON (@p1) without intermediate `Vec<String>`.
    pub fn normalized_keys_json(&self) -> String {
        let n = self.object_entries.len();
        if n == 0 {
            return "[]".into();
        }
        let mut buf = String::with_capacity(n * 32 + 2);
        buf.push('[');
        for (i, entry) in self.object_entries.iter().enumerate() {
            if i > 0 {
                buf.push(',');
            }
            buf.push('"');
            push_json_str(&mut buf, entry.key.as_str());
            buf.push('"');
        }
        buf.push(']');
        buf
    }

    /// JSON array of `{schema,kind,object}` refs for catalog OPENJSON.
    pub fn object_scope_json(&self) -> String {
        let n = self.object_entries.len();
        if n == 0 {
            return "[]".into();
        }
        let mut buf = String::with_capacity(n * 48 + 2);
        buf.push('[');
        for (i, entry) in self.object_entries.iter().enumerate() {
            if i > 0 {
                buf.push(',');
            }
            buf.push_str("{\"schema\":\"");
            push_json_str(&mut buf, entry.key.schema_part());
            buf.push_str("\",\"kind\":\"");
            push_json_str(&mut buf, entry.key.kind_part());
            buf.push_str("\",\"object\":\"");
            push_json_str(&mut buf, entry.key.name_part());
            buf.push_str("\"}");
        }
        buf.push(']');
        buf
    }
}

fn push_json_str(buf: &mut String, s: &str) {
    use std::fmt::Write;
    for c in s.chars() {
        match c {
            '"' => buf.push_str("\\\""),
            '\\' => buf.push_str("\\\\"),
            // Path validation admits C0 controls like tab/newline in Unix
            // filenames; raw controls are invalid JSON and OPENJSON rejects
            // the whole document.
            c if (c as u32) < 0x20 => {
                let _ = write!(buf, "\\u{:04x}", c as u32);
            }
            c => buf.push(c),
        }
    }
}
