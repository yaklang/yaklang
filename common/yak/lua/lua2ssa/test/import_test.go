package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestLuaRequire(t *testing.T) {
	t.Run("require call and local binding", func(t *testing.T) {
		checkSymbols(t, `
local mod = require("mod")
println(mod)
`, "require", "mod")
	})

	t.Run("require in multiple files", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("mod.lua", `
local M = {}
function M.add(a, b)
    return a + b
end
return M
`)
		vf.AddFile("main.lua", `
local mod = require("mod")
println(mod.add(1, 2))
`)
		test.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			require.NotEmpty(t, progs)
			prog := progs[0]
			require.NotEmpty(t, prog.Ref("require"))
			require.NotEmpty(t, prog.Ref("mod"))
			require.NotEmpty(t, prog.Ref("M"))
			return nil
		}, ssaapi.WithLanguage(ssaconfig.LUA), ssaapi.WithMemory())
	})
}
