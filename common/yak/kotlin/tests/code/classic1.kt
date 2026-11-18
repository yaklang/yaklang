package legacy

class Greeter(val name: String) {
    fun greet(): String {
        return "hello " + name
    }
}

object Factory {
    fun create(): Greeter {
        return Greeter("world")
    }
}

fun main() {
    val g = Factory.create()
    println(g.greet())
}
