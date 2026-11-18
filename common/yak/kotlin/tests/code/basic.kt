package com.example

fun add(a: Int, b: Int): Int {
    return a + b
}

fun main() {
    var x = 1
    x = add(x, 2)
    if (x > 2) {
        println(x)
    } else {
        println(0)
    }
    for (i in 0..2) {
        println(i)
    }
    var n = 0
    while (n < 3) {
        n += 1
    }
    when (x) {
        1 -> println("one")
        else -> println("other")
    }
    val list = [1, 2, 3]
    println(list[0])
}
