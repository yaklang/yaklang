package legacy

import scala.collection.mutable._
import scala.util.{Try, Success, Failure}

class Counter(var value: Int) {
  def increment(): Unit = {
    value = value + 1
  }

  def current(): Int = value
}

object Runner {
  def main(args: Array[String]): Unit = {
    val counter = new Counter(0)
    for (i <- 1 to 3) {
      counter.increment()
    }
    var total = 0
    while (total < 3) {
      total = total + 1
    }
    println(counter.current())
  }
}
