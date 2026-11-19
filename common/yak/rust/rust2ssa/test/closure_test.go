package test

import "testing"

func TestRustClosures(t *testing.T) {
	t.Run("captured local is a free value", func(t *testing.T) {
		checkFreeValues(t, `
fn main() {
    let n = 10;
    let target = |x: i32| x + n;
    println(target(1));
}
`, "n")
	})

	t.Run("nested closure captures outer parameter", func(t *testing.T) {
		checkFreeValues(t, `
fn outer(a: i32) -> impl Fn() -> i32 {
    let target = || a;
    target
}
fn main() {
    let f = outer(3);
    println(f());
}
`, "a")
	})

	t.Run("move closure", func(t *testing.T) {
		checkFreeValues(t, `
fn main() {
    let n = 5;
    let target = move || n;
    println(target());
}
`, "n")
	})
}
