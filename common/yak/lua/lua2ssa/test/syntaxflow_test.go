package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestLuaSyntaxFlow(t *testing.T) {
	t.Run("basic dataflow reaches literal", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
local a = 7
println(a)
`, `println(* #-> * as $source)`, map[string][]string{
			"source": {"7"},
		}, ssaapi.WithLanguage(ssaconfig.LUA))
	})

	t.Run("cross function dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
function id(a)
    return a
end
println(id(9))
`, `println(* #-> * as $source)`, map[string][]string{
			"source": {"9"},
		}, ssaapi.WithLanguage(ssaconfig.LUA))
	})

	t.Run("closure free value dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
function make()
    local n = 10
    return function(x)
        return x + n
    end
end
local f = make()
println(f(1))
`, `println(* #-> * as $source)`, map[string][]string{
			"source": {"10"},
		}, ssaapi.WithLanguage(ssaconfig.LUA))
	})

	t.Run("table member dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
local p = {x = 42}
println(p.x)
`, `println(* #-> * as $source)`, map[string][]string{
			"source": {"42"},
		}, ssaapi.WithLanguage(ssaconfig.LUA))
	})
}
