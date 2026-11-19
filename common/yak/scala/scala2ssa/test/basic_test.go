package test

import "testing"

func TestScalaVariables(t *testing.T) {
	t.Run("val and var", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val a = 1
    var b = "two"
    println(a)
    println(b)
  }
}
`, "1", "\"two\"")
	})

	t.Run("assignment updates value", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    a = 2
    println(a)
  }
}
`, "2")
	})

	t.Run("local declaration", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val a = 1
    val b = a + 1
    println(b)
  }
}
`, "2")
	})
}

func TestScalaBasicLiterals(t *testing.T) {
	t.Run("var reassignment", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    a = 2
    println(a)
  }
}
`, "2")
	})

	t.Run("string and boolean literals", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    println("hi")
    println(true)
  }
}
`, "\"hi\"", "true")
	})
}
