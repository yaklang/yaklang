package python

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPythonFieldSensitiveTypeFallbackWithoutCallGraph(t *testing.T) {
	t.Run("positive cmd field", func(t *testing.T) {
		code := `
class Box:
    pass

def run(holder: Box):
    print(holder.cmd)

def assign(cmd):
    holder = Box()
    holder.cmd = cmd
    holder.safe = "safe"
`
		ssatest.CheckSyntaxFlowContain(t, code, `print(* #-> * as $target)`, map[string][]string{
			"target": {"cmd"},
		}, ssaapi.WithLanguage(ssaconfig.PYTHON))
	})

	t.Run("negative safe field", func(t *testing.T) {
		code := `
class Box:
    pass

def run(holder: Box):
    print(holder.safe)

def assign(cmd):
    holder = Box()
    holder.cmd = cmd
    holder.safe = "safe"
`
		ssatest.CheckSyntaxFlowContain(t, code, `print(* #-> * as $target)`, map[string][]string{
			"target": {"safe"},
		}, ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}
