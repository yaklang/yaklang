package test

import "testing"

func TestRustCFGIfPhi(t *testing.T) {
	t.Run("if branch", func(t *testing.T) {
		checkPhi(t, `
fn main() {
    let mut a = 1;
    if c {
        a = 2;
    }
    println(a);
}
`, "a", "1", "2")
	})

	t.Run("if else branch", func(t *testing.T) {
		checkPhi(t, `
fn main() {
    let mut a = 1;
    if c {
        a = 2;
    } else {
        a = 3;
    }
    println(a);
}
`, "a", "2", "3")
	})

	t.Run("else if chain", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let mut a = 1;
    if c {
        a = 2;
    } else if d {
        a = 3;
    } else {
        a = 4;
    }
    println(a);
}
`, "phi(a)", "2", "3", "4")
	})

	t.Run("nested if", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let mut a = 1;
    if c {
        if d {
            a = 3;
        } else {
            a = 2;
        }
    }
    println(a);
}
`, "phi(a)", "3", "2", "1")
	})

	t.Run("if without else", func(t *testing.T) {
		checkValues(t, `
fn main() {
    let mut a = 1;
    if c {
        a = 2;
    }
    println(a);
}
`, "phi(a)[2,1]")
	})
}

func TestRustCFGLoops(t *testing.T) {
	t.Run("while loop", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let mut a = 1;
    while c {
        a = a + 1;
    }
    println(a);
}
`, "phi(a)", "add(a, 1)")
	})

	t.Run("while with break", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let mut a = 1;
    while c {
        a = 2;
        break;
    }
    println(a);
}
`, "phi(a)", "2", "1")
	})

	t.Run("infinite loop with break", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let mut a = 1;
    loop {
        a = 2;
        if c {
            break;
        }
        a = 3;
    }
    println(a);
}
`, "phi(a)")
	})

	t.Run("for range", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let mut a = 1;
    for i in 0..3 {
        a = a + i;
    }
    println(a);
}
`, "phi(a)")
	})

	t.Run("continue and break", func(t *testing.T) {
		checkValuesContain(t, `
fn main() {
    let mut a = 1;
    for i in 0..3 {
        if i == 1 {
            continue;
        }
        if i == 2 {
            break;
        }
        a = a + i;
    }
    println(a);
}
`, "phi(a)")
	})
}
