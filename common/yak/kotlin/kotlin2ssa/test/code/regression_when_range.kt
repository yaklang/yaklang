@file:Suppress("unused")

value class Id(val raw: Int)

fun classify(value: Any): Int {
    when (val subject = value) {
        is List<*> -> return 1
        in 0..<3 -> return 2
        else -> {
            val unsigned = 1u
            val text = $$"price: $5"
            return subject.hashCode() + unsigned.toInt() + text.length
        }
    }
}

fun describe(value: Any) {
    println($$"value: $${value}")
}

fun <T> requireNonNull(value: T & Any): T & Any = value
