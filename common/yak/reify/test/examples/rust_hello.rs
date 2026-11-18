// Build from the worktree root:
// RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"
// export TMPDIR=/mnt/data/cache/go-tmp
// rustc --edition=2021 -C opt-level=0 -C overflow-checks=off -C symbol-mangling-version=v0 -o "$RUN/rust_hello" common/yak/reify/test/examples/rust_hello.rs
// Keep symbols: unlike Go, stripping Rust names prevents this layer from recovering them.
#[inline(never)]
fn add(a: i64, b: i64) -> i64 {
    a + b
}

fn main() {
    println!("hello {}", add(40, 2));
}
