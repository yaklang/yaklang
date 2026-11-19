package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestKotlinMultiFileProject(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("util.kt", `
fun helper(a: Int): Int {
    return a + 1
}
`)
	vf.AddFile("main.kt", `
fun main() {
    val value = helper(1)
    println(value)
}
`)
	test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		require.NotEmpty(t, progs)
		prog := progs[0]
		require.NotEmpty(t, prog.Ref("main"))
		require.NotEmpty(t, prog.Ref("helper"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.KOTLIN), ssaapi.WithMemory())
}

func TestKotlinProjectLocal(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app.kt", `
fun id(a: Int): Int {
    return a
}
fun main() {
    println(id(1))
}
`)
	test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		require.NotEmpty(t, progs)
		require.NotEmpty(t, progs[0].Ref("id"))
		require.NotEmpty(t, progs[0].Ref("main"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.KOTLIN), ssaapi.WithMemory())
}
