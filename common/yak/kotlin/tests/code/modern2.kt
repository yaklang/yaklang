package modern

data class Point(val x: Int, val y: Int)

sealed interface Shape {
    data class Circle(val radius: Int) : Shape
    data class Square(val side: Int) : Shape
}

fun area(shape: Shape): Int = when (shape) {
    is Shape.Circle -> 3 * shape.radius * shape.radius
    is Shape.Square -> shape.side * shape.side
}

fun main() {
    val p = Point(1, 2)
    val values = listOf(
        p,
        Point(3, 4),
    )
    println(values.size)
}
