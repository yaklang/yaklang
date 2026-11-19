package test

import "testing"

func TestRustStructs(t *testing.T) {
	t.Run("struct literal and field", func(t *testing.T) {
		checkValuesContain(t, `
struct Point {
    x: i32,
    y: i32,
}
fn main() {
    let p = Point { x: 1, y: 2 };
    println(p.x);
}
`, "p.x")
	})

	t.Run("struct field assignment", func(t *testing.T) {
		checkValuesContain(t, `
struct Box {
    value: i32,
}
fn main() {
    let mut b = Box { value: 1 };
    b.value = 7;
    println(b.value);
}
`, "7")
	})

	t.Run("struct method syntax", func(t *testing.T) {
		checkSymbols(t, `
struct Counter {
    value: i32,
}
impl Counter {
    fn get(&self) -> i32 {
        self.value
    }
}
fn main() {
    let c = Counter { value: 1 };
    c.get();
}
`, "main")
	})

	t.Run("tuple and array member", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let pair = (1, 2);
    let array = [3, 4];
    println(pair.0);
    println(array[1]);
}
`, "pair", "array")
	})
}
