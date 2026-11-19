package test

import "testing"

func TestScalaTypeCast(t *testing.T) {
	t.Run("asInstanceOf uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val a = 1
    val target = a.asInstanceOf[Long]
    println(target)
  }
}
`, "target")
	})

	t.Run("ascription uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val a = 1
    val target = (a: Long)
    println(target)
  }
}
`, "target")
	})

	t.Run("typecast keeps source operand", func(t *testing.T) {
		checkPrintlnValueContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val a = 1
    val target = a.asInstanceOf[Long]
    println(target)
  }
}
`, "castType(number", "1")
	})
}
