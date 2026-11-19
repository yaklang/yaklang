package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestScalaMultiFileProject(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("util.scala", `
object Util {
  def helper(a: Int): Int = {
    a + 1
  }
}
`)
	vf.AddFile("main.scala", `
object Main {
  def main(args: Array[String]): Unit = {
    val value = Util.helper(1)
    println(value)
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
}

func TestScalaProjectLocal(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app.scala", `
object Main {
  def id(a: Int): Int = {
    a
  }
  def main(args: Array[String]): Unit = {
    println(id(1))
  }
}
`)
	test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		require.NotEmpty(t, progs)
		require.NotEmpty(t, progs[0].Ref("id"))
		require.NotEmpty(t, progs[0].Ref("main"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.SCALA), ssaapi.WithMemory())
}
