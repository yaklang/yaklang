package modern

enum Color {
  case Red
  case Green
  case Blue
}

case class Point(x: Int, y: Int)

def area(p: Point): Int =
  p.x * p.y

def main(): Unit =
  val p = Point(2, 3)
  println(area(p))
  val color = Color.Red
  println(color)
