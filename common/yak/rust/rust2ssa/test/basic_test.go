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
