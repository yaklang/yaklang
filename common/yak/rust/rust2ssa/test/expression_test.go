package test

import "testing"

func TestRustExpressions(t *testing.T) {
	t.Run("integer arithmetic", func(t *testing.T) {
		checkValues(t, `
fn main() {
    println(1 + 2);
    println(7 - 3);
    println(2 * 3);
    println(8 / 2);
    println(7 % 3);
}
`, "3", "4", "6", "4", "1")
	})

	t.Run("unary operators", func(t *testing.T) {
		checkValues(t, `
fn main() {
    println(-1);
    println(!false);
}
`, "-1", "true")
	})

	t.Run("comparison operators", func(t *testing.T) {
		checkValues(t, `
fn main() {
    println(1 < 2);
    println(2 <= 2);
    println(3 == 3);
    println(3 != 4);
    println(4 > 3);
}
`, "true", "true", "true", "true", "true")
	})

	t.Run("logical operators", func(t *testing.T) {
		checkValues(t, `
fn main() {
    println(true && false);
    println(false || true);
}
`, "false", "true")
	})

	t.Run("bitwise operators", func(t *testing.T) {
		checkValues(t, `
fn main() {
    println(1 | 2);
    println(7 & 3);
    println(1 ^ 3);
    println(1 << 2);
    println(8 >> 1);
}
`, "3", "3", "2", "4", "4")
	})

	t.Run("string literal", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    println("hello");
}
`, "hello")
	})

	t.Run("boolean literals", func(t *testing.T) {
		checkValues(t, `
fn main() {
    println(true);
    println(false);
}
`, "true", "false")
	})

	t.Run("array index", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let a = [10, 20, 30];
    println(a[1]);
}
`, "20")
	})

	t.Run("struct field access", func(t *testing.T) {
		checkValuesContain(t, `
struct Point {
    x: i32,
}
fn main() {
    let p = Point { x: 42 };
    println(p.x);
}
`, "42")
	})

	t.Run("if expression", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let a = if c { 1 } else { 2 };
    println(a);
}
`, "1", "2")
	})

	t.Run("range expression", func(t *testing.T) {
		checkSymbols(t, `
fn main() {
    let a = 0..3;
    println(a);
}
`, "main", "a")
	})
}

func TestRustShortCircuit(t *testing.T) {
	t.Run("false and short circuits", func(t *testing.T) {
		checkPrintlnValueContain(t, `
fn side() -> bool { return true; }
fn main() {
    println(false && side());
}
`, "false")
	})

	t.Run("true or short circuits", func(t *testing.T) {
		checkPrintlnValueContain(t, `
fn side() -> bool { return false; }
fn main() {
    println(true || side());
}
`, "true")
	})
}
