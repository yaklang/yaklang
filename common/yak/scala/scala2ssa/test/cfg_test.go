package test

import "testing"

func TestScalaCFGIfPhi(t *testing.T) {
	t.Run("if branch", func(t *testing.T) {
		checkPhi(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    if (c) {
      a = 2
    }
    println(a)
  }
}
`, "a", "1", "2")
	})

	t.Run("if else branch", func(t *testing.T) {
		checkPhi(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    if (c) {
      a = 2
    } else {
      a = 3
    }
    println(a)
  }
}
`, "a", "2", "3")
	})

	t.Run("else if chain", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    if (c) {
      a = 2
    } else if (d) {
      a = 3
    } else {
      a = 4
    }
    println(a)
  }
}
`, "phi(a)", "2", "3", "4")
	})

	t.Run("nested if", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    if (c) {
      if (d) {
        a = 3
      } else {
        a = 2
      }
    }
    println(a)
  }
}
`, "phi(a)", "3", "2", "1")
	})

	t.Run("if without else", func(t *testing.T) {
		checkValues(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    if (c) {
      a = 2
    }
    println(a)
  }
}
`, "phi(a)[2,1]")
	})
}

func TestScalaCFGLoops(t *testing.T) {
	t.Run("while loop", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    while (c) {
      a = a + 1
    }
    println(a)
  }
}
`, "phi(a)", "add(a, 1)")
	})

	t.Run("for range", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    for (i <- 1 to 3) {
      a = a + i
    }
    println(a)
  }
}
`, "phi(a)")
	})

	t.Run("for until", func(t *testing.T) {
		checkValuesContain(t, `
object Main {
  def main(args: Array[String]): Unit = {
    var a = 1
    for (i <- 1 until 3) {
      a = a + i
    }
    println(a)
  }
}
`, "phi(a)")
	})
}

func TestScalaMatch(t *testing.T) {
	t.Run("literal cases select first match", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val x = 2
    val r = x match {
      case 1 => 10
      case 2 => 20
      case _ => 30
    }
    println(r)
  }
}
`, "20")
	})

	t.Run("guard participates in matching", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val x = 5
    val r = x match {
      case n if n > 3 => 1
      case _ => 0
    }
    println(r)
  }
}
`, "1")
	})

	t.Run("binding pattern keeps subject", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val x = 4
    val r = x match {
      case n => n
    }
    println(r)
  }
}
`, "4")
	})

	t.Run("case block body", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val x = 1
    val r = x match {
      case 1 => {
        val y = 3
        y + 1
      }
      case _ => 0
    }
    println(r)
  }
}
`, "4")
	})
}

func TestScalaMatchPatterns(t *testing.T) {
	t.Run("or pattern", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val x = 2
    val r = x match {
      case 1 | 2 => 5
      case _ => 0
    }
    println(r)
  }
}
`, "5")
	})

	t.Run("guard falls through", func(t *testing.T) {
		checkPrintlnValue(t, `
object Main {
  def main(args: Array[String]): Unit = {
    val x = 1
    val r = x match {
      case n if n > 3 => 1
      case _ => 0
    }
    println(r)
  }
}
`, "0")
	})
}
