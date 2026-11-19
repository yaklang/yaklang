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
