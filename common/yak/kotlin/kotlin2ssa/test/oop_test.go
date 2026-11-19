package test

import "testing"

func TestKotlinClassesAndObjects(t *testing.T) {
	t.Run("class properties and methods", func(t *testing.T) {
		checkSymbols(t, `
class Box {
    val value = 1
    fun get(): Int {
        return value
    }
}
fun main() {
    get()
}
`, "value", "get", "main")
	})

	t.Run("class method body", func(t *testing.T) {
		checkValues(t, `
class Counter {
    var count = 1
    fun next(): Int {
        println(count)
        count = count + 1
        return count
    }
}
fun main() {
    next()
}
`, "1")
	})

	t.Run("object declaration", func(t *testing.T) {
		checkSymbols(t, `
object Config {
    val name = "demo"
    fun show() {
        println(name)
    }
}
fun main() {
    show()
}
`, "name", "show", "main")
	})

	t.Run("data class", func(t *testing.T) {
		checkSymbols(t, `
data class User(val name: String, val age: Int)
fun main() {
    val u = User("a", 1)
    println(u.name)
}
`, "main", "u")
	})

	t.Run("inheritance syntax", func(t *testing.T) {
		checkSymbols(t, `
open class Base {
    fun base() {}
}
class Child : Base() {
    fun child() {}
}
fun main() {
    child()
}
`, "base", "child", "main")
	})

	t.Run("companion object", func(t *testing.T) {
		checkSymbols(t, `
class Holder {
    companion object {
        fun make(): Int = 1
    }
}
fun main() {
    make()
}
`, "main")
	})
}

func TestKotlinThisField(t *testing.T) {
	t.Run("this field read", func(t *testing.T) {
		checkPrintlnValueContain(t, `
class Box {
    val value = 42
    fun fetch(): Int {
        println(this.value)
        return this.value
    }
}
fun main() {
    fetch()
}
`, "42")
	})

	t.Run("this field write", func(t *testing.T) {
		checkPrintlnValueContain(t, `
class Counter {
    var value = 1
    fun bump(): Int {
        this.value = this.value + 1
        println(this.value)
        return this.value
    }
}
fun main() {
    bump()
}
`, "2")
	})
}
