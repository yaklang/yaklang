package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestLuaMultiFileProject(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("util.lua", `
function helper(a)
    return a + 1
end
`)
	vf.AddFile("main.lua", `
function main()
    local value = helper(1)
    println(value)
end
main()
`)

	test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		require.NotEmpty(t, progs)
		prog := progs[0]
		require.NotEmpty(t, prog.Ref("helper"))
		require.NotEmpty(t, prog.Ref("main"))
		require.NotEmpty(t, prog.Ref("println"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.LUA), ssaapi.WithMemory())
}

func TestLuaProjectLocal(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("app.lua", `
local function id(a)
    return a
end
println(id(1))
`)
	test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		require.NotEmpty(t, progs)
		require.NotEmpty(t, progs[0].Ref("id"))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.LUA), ssaapi.WithMemory())
}
