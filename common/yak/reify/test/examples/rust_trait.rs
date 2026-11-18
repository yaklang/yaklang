// Build from the worktree root. Output must stay under /mnt/data.
// RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"
// export TMPDIR=/mnt/data/cache/go-tmp
// rustc --edition=2021 -C opt-level=0 -C overflow-checks=off -C symbol-mangling-version=v0 -o "$RUN/rust_trait" common/yak/reify/test/examples/rust_trait.rs
//
// The call through &dyn Shape loads Area from a vtable, so the concrete target
// is not a direct call edge.
trait Shape {
    fn area(&self) -> i64;
}

struct Square {
    side: i64,
}

impl Shape for Square {
    #[inline(never)]
    fn area(&self) -> i64 {
        self.side * self.side
    }
}

#[inline(never)]
fn total(shape: &dyn Shape) -> i64 {
    shape.area()
}

fn main() {
    println!("{}", total(&Square { side: 6 }));
}