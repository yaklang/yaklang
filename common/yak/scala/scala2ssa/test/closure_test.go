package test

import "testing"

func TestScalaClosures(t *testing.T) {
	t.Run("captured local is a free value", func(t *testing.T) {
		checkFreeValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val n = 10
    val target = { (x: Int) => x + n }
    println(target(1))
  }
}
`, "n")
	})

	t.Run("nested closure captures outer parameter", func(t *testing.T) {
		checkFreeValues(t, `
object Main {
  def outer(a: Int): Int => Int = {
    val target = { (x: Int) => x + a }
    target
  }
  def main(args: Array[String]): Unit = {
    val f = outer(3)
    println(f(1))
  }
}
`, "a")
	})
}
