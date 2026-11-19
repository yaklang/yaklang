package test

import "testing"

func TestRustFunctions(t *testing.T) {
	t.Run("parameters", func(t *testing.T) {
		checkValues(t, `
fn add(a: i32, b: i32) {
    println(a);
    println(b);
}
fn main() {
    add(1, 2);
}
`, "Parameter-a", "Parameter-b")
	})

	t.Run("function definitions are exported", func(t *testing.T) {
		checkSymbols(t, `
fn helper(a: i32) -> i32 {
    a
}
fn main() {
    helper(1);
}
`, "helper", "main")
	})

	t.Run("return value flows to call", func(t *testing.T) {
		checkValuesContain(t, `
fn identity(a: i32) -> i32 {
    a
}
fn main() {
    println(identity(7));
}
`, "identity", "7")
	})

	t.Run("recursive function", func(t *testing.T) {
		checkSymbols(t, `
fn fact(n: i32) -> i32 {
    if n <= 1 {
        return 1;
    }
    n * fact(n - 1)
}
fn main() {
    println(fact(3));
}
`, "fact", "main")
	})

	t.Run("function pointer parameter", func(t *testing.T) {
		checkSymbols(t, `
fn apply(f: fn(i32) -> i32, x: i32) -> i32 {
    f(x)
}
fn id(x: i32) -> i32 {
    x
}
fn main() {
    apply(id, 1);
}
`, "apply", "id")
	})

	t.Run("method syntax is parsed", func(t *testing.T) {
		checkSymbols(t, `
struct Point {
    x: i32,
}
impl Point {
    fn get(&self) -> i32 {
        self.x
    }
}
fn main() {
    let p = Point { x: 1 };
    p.get();
}
`, "main")
	})
}

func TestRustCallSiteOperand(t *testing.T) {
	checkPrintlnValueRegexp(t, `
fn identity(a: i32) -> i32 {
    a
}
fn main() {
    println(identity(7));
}
`, `identity\(7\)`)
}
