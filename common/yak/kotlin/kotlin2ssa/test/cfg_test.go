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

func TestKotlinWhen(t *testing.T) {
	t.Run("value when selects first match", func(t *testing.T) {
		checkPrintlnValue(t, `
fun main() {
    val x = 2
    val r = when (x) {
        1 -> 10
        2 -> 20
        else -> 30
    }
    println(r)
}
`, "20")
	})

	t.Run("when without subject", func(t *testing.T) {
		checkPrintlnValue(t, `
fun main() {
    val x = 5
    val r = when {
        x > 3 -> 1
        else -> 0
    }
    println(r)
}
`, "1")
	})

	t.Run("when branch block", func(t *testing.T) {
		checkPrintlnValue(t, `
fun main() {
    val x = 1
    val r = when (x) {
        1 -> {
            val y = 3
            y + 1
        }
        else -> 0
    }
    println(r)
}
`, "4")
	})

	t.Run("when as statement", func(t *testing.T) {
		checkPrintlnValue(t, `
fun main() {
    val x = 1
    when (x) {
        1 -> println(1)
        else -> println(0)
    }
}
`, "1")
	})
}

func TestKotlinWhenConditions(t *testing.T) {
	t.Run("multiple conditions in one entry", func(t *testing.T) {
		checkPrintlnValue(t, `
fun main() {
    val x = 2
    val r = when (x) {
        1, 2 -> 5
        else -> 0
    }
    println(r)
}
`, "5")
	})

	t.Run("first matching entry wins", func(t *testing.T) {
		checkPrintlnValue(t, `
fun main() {
    val x = 1
    val r = when (x) {
        1 -> 10
        1 -> 20
        else -> 30
    }
    println(r)
}
`, "10")
	})
}
