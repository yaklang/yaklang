package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestDeepChain(t *testing.T) {
	ssatest.Check(t, `a.b.c.d.e.f.g.h().aaa.bbb.ccc()`, func(prog *ssaapi.Program) error {
		if results := prog.SyntaxFlowChain(`a...h()...ccc()`); results.Show().Len() <= 0 {
			return utils.Error("failed to match all of the substring, bad dot graph")
		}
		return nil
	})
}

func TestDeepChainGLob(t *testing.T) {
	ssatest.Check(t, `a.b.c.d.e.f.g.h().aaa.bbb.ccc()`, func(prog *ssaapi.Program) error {
		if results := prog.SyntaxFlowChain(`a...a*...c*()`); results.Show().Len() <= 0 {
			return utils.Error("failed to match all of the substring, bad dot graph")
		}
		return nil
	})
}

func TestDeepChainRegexp(t *testing.T) {
	ssatest.Check(t, `a.b.c.d.e.f.g.h().aaa.bbb.ccc()`, func(prog *ssaapi.Program) error {
		if results := prog.SyntaxFlowChain(`a.../a[a-z]+/.../c[a-z]+/()`); results.Show().Len() <= 0 {
			return utils.Error("failed to match all of the substring, bad dot graph")
		}
		return nil
	})
}
