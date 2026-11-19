package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestScalaImports(t *testing.T) {
	t.Run("import declaration", func(t *testing.T) {
		checkSymbols(t, `
import scala.collection.mutable
object Main {
  def main(args: Array[String]): Unit = {
    println(1)
  }
}
`, "main")
	})

	t.Run("package declaration", func(t *testing.T) {
		test.CheckWithNameOnlyInMemory("", t, `
package com.example
object Main {
  def main(args: Array[String]): Unit = {
    println(1)
  }
}
`, func(prog *ssaapi.Program) error {
			require.Equal(t, "com.example", prog.Program.PkgName)
			require.NotEmpty(t, prog.Ref("main"))
			return nil
		}, ssaapi.WithLanguage(ssaconfig.SCALA), ssaapi.WithMemory())
	})

	t.Run("multi file import", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("util.scala", `
package util
object Util {
  def helper(a: Int): Int = {
    a + 1
  }
}
`)
		vf.AddFile("main.scala", `
package app
import util.Util
object Main {
  def main(args: Array[String]): Unit = {
    Util.helper(1)
  }
}
`)
		test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			require.NotEmpty(t, progs)
			prog := progs[0]
			require.NotEmpty(t, prog.Ref("main"))
			require.NotEmpty(t, prog.Ref("helper"))
			return nil
		}, ssaapi.WithLanguage(ssaconfig.SCALA), ssaapi.WithMemory())
	})
}
