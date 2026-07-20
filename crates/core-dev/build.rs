use std::process::Command;

fn main() {
    // Footprint baselines are ABI-specific; stamp the compiling target and
    // rustc so baseline JSON carries real provenance instead of "unknown".
    let target = std::env::var("TARGET").unwrap_or_else(|_| "unknown".into());
    println!("cargo:rustc-env=RMIG_BUILD_TARGET={target}");
    let rustc = std::env::var("RUSTC").unwrap_or_else(|_| "rustc".into());
    let version = Command::new(rustc)
        .arg("--version")
        .output()
        .ok()
        .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| "unknown".into());
    println!("cargo:rustc-env=RMIG_RUSTC_VERSION={version}");
}
