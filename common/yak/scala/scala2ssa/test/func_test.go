package test

import "testing"

func TestScalaFunctions(t *testing.T) {
	t.Run("parameters", func(t *testing.T) {
		checkValues(t, `
object Main {
  def add(a: Int, b: Int): Unit = {
    println(a)
    println(b)
  }
  def main(args: Array[String]): Unit = {
    add(1, 2)
  }
}
`, "Parameter-a", "Parameter-b")
	})

	t.Run("function symbols", func(t *testing.T) {
		checkSymbols(t, `
def helper(a: Int): Int = a
object Main {
  def main(args: Array[String]): Unit = {
    helper(1)
  }
}
`, "helper", "main")
	})

	t.Run("return value", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def identity(a: Int): Int = {
    return a
  }
  def main(args: Array[String]): Unit = {
    println(identity(7))
  }
}
`, "identity", "7")
	})

	t.Run("expression body", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def add(a: Int, b: Int): Int = a + b
  def main(args: Array[String]): Unit = {
    println(add(1, 2))
  }
}
`, "add", "1", "2")
	})

	t.Run("recursive function", func(t *testing.T) {
		checkSymbols(t, `
object Main {
  def fact(n: Int): Int = {
    if (n <= 1) {
      return 1
    }
    n * fact(n - 1)
  }
  def main(args: Array[String]): Unit = {
    println(fact(3))
  }
}
`, "fact", "main")
	})

	t.Run("anonymous function", func(t *testing.T) {
		checkSymbols(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val f = (x: Int) => x + 1
    println(f(1))
  }
}
`, "main", "f")
	})
}

func TestScalaCallSiteOperand(t *testing.T) {
	checkPrintlnValueRegexp(t, `
object Main {
  def identity(a: Int): Int = {
    return a
  }
  def main(args: Array[String]): Unit = {
    println(identity(7))
  }
}
`, `identity\(7\)`)
}
