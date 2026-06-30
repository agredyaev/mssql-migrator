use super::super::Workspace;

impl Workspace {
    /// Returns all normalized object keys (`schema/kind/name`) in workspace order.
    pub fn normalized_keys(&self) -> Vec<String> {
        self.object_entries
            .iter()
            .enumerate()
            .map(|(i, o)| o.key_str(self, i).to_string())
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
            let k = entry.key_str(self, i);
            for c in k.chars() {
                if c == '"' || c == '\\' {
                    buf.push('\\');
                }
                buf.push(c);
            }
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
            push_json_str(&mut buf, entry.schema_part(self, i));
            buf.push_str("\",\"kind\":\"");
            push_json_str(&mut buf, entry.kind_part(self, i));
            buf.push_str("\",\"object\":\"");
            push_json_str(&mut buf, entry.name_part(self, i));
            buf.push_str("\"}");
        }
        buf.push(']');
        buf
    }
}

fn push_json_str(buf: &mut String, s: &str) {
    for c in s.chars() {
        if c == '"' || c == '\\' {
            buf.push('\\');
        }
        buf.push(c);
    }
}
