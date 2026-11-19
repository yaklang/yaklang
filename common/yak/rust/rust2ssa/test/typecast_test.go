package test

import "testing"

func TestRustTypeCast(t *testing.T) {
	t.Run("as int uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
fn main() {
    let a = 1;
    let target = a as i64;
    println(target);
}
`, "target")
	})

	t.Run("as float uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
fn main() {
    let a = 1;
    let target = a as f64;
    println(target);
}
`, "target")
	})

	t.Run("typecast keeps source operand", func(t *testing.T) {
		checkPrintlnValueContain(t, `
fn main() {
    let a = 1;
    let target = a as i64;
    println(target);
}
`, "castType(number", "1")
	})
}
