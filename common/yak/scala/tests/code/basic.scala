package com.example

object Main {
  def add(a: Int, b: Int): Int = {
    a + b
  }

  def main(args: Array[String]): Unit = {
    var x: Int = 1
    x = add(x, 2)
    if (x > 2) {
      println(x)
    } else {
      println(0)
    }
    while (x < 10) {
      x = x + 1
    }
    for (i <- 1 to 3) {
      println(i)
    }
    val list = List(1, 2, 3)
    println(list)
  }
}
