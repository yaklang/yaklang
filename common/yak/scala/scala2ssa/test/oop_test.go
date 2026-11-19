package test

import "testing"

func TestScalaClassesAndObjects(t *testing.T) {
	t.Run("class methods", func(t *testing.T) {
		checkSymbols(t, `
class Box {
  val value = 1
  def get(): Int = {
    return value
  }
}
object Main {
  def main(args: Array[String]): Unit = {
    get()
  }
}
`, "value", "get", "main")
	})

	t.Run("case class", func(t *testing.T) {
		checkSymbols(t, `
case class User(name: String, age: Int)
object Main {
  def main(args: Array[String]): Unit = {
    val u = User("a", 1)
    println(u.name)
  }
}
`, "main", "u")
	})

	t.Run("object methods", func(t *testing.T) {
		checkSymbols(t, `
object Util {
  def add(a: Int, b: Int): Int = {
    a + b
  }
}
object Main {
  def main(args: Array[String]): Unit = {
    Util.add(1, 2)
  }
}
`, "add", "main")
	})

	t.Run("trait and inheritance syntax", func(t *testing.T) {
		checkSymbols(t, `
trait Base {
  def base(): Int = 1
}
class Child extends Base {
  def base(): Int = 1
}
object Main {
  def main(args: Array[String]): Unit = {
    new Child().base()
  }
}
`, "base", "main")
	})

	t.Run("class field assignment", func(t *testing.T) {
		checkValuesContain(t, `
class Counter {
  var value = 1
  def next(): Int = {
    println(value)
    value = value + 1
    return value
  }
}
object Main {
  def main(args: Array[String]): Unit = {
    next()
  }
}
`, "1")
	})

	t.Run("given and extension syntax", func(t *testing.T) {
		checkSymbols(t, `
given intOrdering: Ordering[Int] with {
  def compare(x: Int, y: Int): Int = x - y
}
object Main {
  def main(args: Array[String]): Unit = {
    println(compare(1, 2))
  }
}
`, "main")
	})
}
