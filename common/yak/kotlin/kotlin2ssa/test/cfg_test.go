package test

import "testing"

func TestKotlinCFGIfPhi(t *testing.T) {
	t.Run("if branch", func(t *testing.T) {
		checkPhi(t, `
fun main() {
    var a = 1
    if (c) {
        a = 2
    }
    println(a)
}
`, "a", "1", "2")
	})

	t.Run("if else branch", func(t *testing.T) {
		checkPhi(t, `
fun main() {
    var a = 1
    if (c) {
        a = 2
    } else {
        a = 3
    }
    println(a)
}
`, "a", "2", "3")
	})

	t.Run("else if chain", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    var a = 1
    if (c) {
        a = 2
    } else if (d) {
        a = 3
    } else {
        a = 4
    }
    println(a)
}
`, "phi(a)", "2", "3", "4")
	})

	t.Run("nested if", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    var a = 1
    if (c) {
        if (d) {
            a = 3
        } else {
            a = 2
        }
    }
    println(a)
}
`, "phi(a)", "3", "2", "1")
	})

	t.Run("if without else", func(t *testing.T) {
		checkValues(t, `
fun main() {
    var a = 1
    if (c) {
        a = 2
    }
    println(a)
}
`, "phi(a)[2,1]")
	})

	t.Run("if with return", func(t *testing.T) {
		checkValues(t, `
fun choose(c: Boolean): Int {
    var a = 1
    if (c) {
        return 2
    }
    println(a)
    return a
}
`, "1")
	})
}

func TestKotlinCFGLoops(t *testing.T) {
	t.Run("while loop", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    var a = 1
    while (c) {
        a = a + 1
    }
    println(a)
}
`, "phi(a)", "add(a, 1)")
	})

	t.Run("do while loop", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    var a = 1
    do {
        a = a + 1
    } while (c)
    println(a)
}
`, "phi(a)")
	})

	t.Run("for range", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    var a = 1
    for (i in 0..2) {
        a = a + i
    }
    println(a)
}
`, "phi(a)")
	})

	t.Run("break and continue", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    var a = 1
    for (i in 0..2) {
        if (i == 1) {
            continue
        }
        if (i == 2) {
            break
        }
        a = a + i
    }
    println(a)
}
`, "phi(a)")
	})

	t.Run("when branch", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    var a = 1
    when (c) {
        1 -> a = 2
        else -> a = 3
    }
    println(a)
}
`, "phi(a)", "2", "3")
	})
}
