package test

import "testing"

func TestRustVariables(t *testing.T) {
	t.Run("let binding", func(t *testing.T) {
		checkValues(t, `
fn main() {
    let a = 1;
    let b = "two";
    println(a);
    println(b);
}
`, "1", "\"two\"")
	})

	t.Run("mutable assignment", func(t *testing.T) {
		checkValues(t, `
fn main() {
    let mut a = 1;
    a = 2;
    println(a);
}
`, "2")
	})

	t.Run("shadowing", func(t *testing.T) {
		checkValues(t, `
fn main() {
    let a = 1;
    let a = 2;
    println(a);
}
`, "2")
	})
}

func TestRustBasicLiterals(t *testing.T) {
	t.Run("const and static", func(t *testing.T) {
		checkPrintlnValue(t, `
const MAX: i32 = 10;
static BASE: i32 = 3;
fn main() {
    println(MAX);
    println(BASE);
}
`, "10", "3")
	})

	t.Run("array index write", func(t *testing.T) {
		checkPrintlnValue(t, `
fn main() {
    let mut a = [1, 2, 3];
    a[1] = 9;
    println(a[1]);
}
`, "9")
	})

	t.Run("nested struct field", func(t *testing.T) {
		checkPrintlnValue(t, `
struct Inner { value: i32 }
struct Outer { inner: Inner }
fn main() {
    let o = Outer { inner: Inner { value: 4 } };
    println(o.inner.value);
}
`, "4")
	})
}
