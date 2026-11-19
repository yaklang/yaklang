package test

import "testing"

func TestScalaExpressions(t *testing.T) {
	t.Run("integer arithmetic", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println(1 + 2)
    println(7 - 3)
    println(2 * 3)
    println(8 / 2)
    println(7 % 3)
  }
}
`, "3", "4", "6", "4", "1")
	})

	t.Run("unary operators", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println(-1)
    println(!false)
  }
}
`, "-1", "true")
	})

	t.Run("comparison operators", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println(1 < 2)
    println(2 <= 2)
    println(3 == 3)
    println(3 != 4)
    println(4 > 3)
  }
}
`, "true", "true", "true", "true", "true")
	})

	t.Run("logical operators", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println(true && false)
    println(false || true)
  }
}
`, "false", "true")
	})

	t.Run("bitwise operators", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println(1 | 2)
    println(7 & 3)
    println(1 ^ 3)
    println(1 << 2)
    println(8 >> 1)
  }
}
`, "3", "3", "2", "4", "4")
	})

	t.Run("string literal", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println("hello")
  }
}
`, "hello")
	})

	t.Run("string concatenation", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println("a" + "b")
  }
}
`, "ab")
	})

	t.Run("char literal", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println('x')
  }
}
`, "x")
	})

	t.Run("boolean literals", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println(true)
    println(false)
  }
}
`, "true", "false")
	})

	t.Run("hex literal", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println(0xFF)
  }
}
`, "255")
	})

	t.Run("if expression", func(t *testing.T) {
		checkSymbols(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val a = if (c) 1 else 2
    println(a)
  }
}
`, "main", "a")
	})

	t.Run("string interpolation", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val name = "demo"
    println(s"hello $name")
  }
}
`, "hello")
	})
}

func TestScalaShortCircuit(t *testing.T) {
	t.Run("false and short circuits", func(t *testing.T) {
		checkPrintlnValueContain(t, `
object Main {
  def side(): Boolean = { return true }
  def main(args: Array[String]): Unit = {
    println(false && side())
  }
}
`, "false")
	})

	t.Run("true or short circuits", func(t *testing.T) {
		checkPrintlnValueContain(t, `
object Main {
  def side(): Boolean = { return false }
  def main(args: Array[String]): Unit = {
    println(true || side())
  }
}
`, "true")
	})
}
