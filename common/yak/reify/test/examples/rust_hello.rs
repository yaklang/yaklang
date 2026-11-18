// Build from the worktree root:
// RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"
// export TMPDIR=/mnt/data/cache/go-tmp
// rustc --edition=2021 -C opt-level=0 -C overflow-checks=off -C symbol-mangling-version=v0 -o "$RUN/rust_hello" common/yak/reify/test/examples/rust_hello.rs
// Add -C relocation-model=static -C link-arg=-no-pie when decoding GREETING:
// PIE keeps the descriptor pointer in a relocation, which reify does not apply.
// Keep symbols: unlike Go, stripping Rust names prevents this layer from recovering them.
// GREETING is a static fat pointer: data address and length. The strings
// command reads it at the GREETING symbol.
#[no_mangle]
pub static GREETING: &str = "hello from reify";

#[inline(never)]
fn add(a: i64, b: i64) -> i64 {
    a + b
}

fn main() {
    println!("{} {}", GREETING, add(40, 2));
}
