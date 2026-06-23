package typescript

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestTSFieldSensitiveTypeFallbackWithoutCallGraph(t *testing.T) {
	t.Run("positive cmd field", func(t *testing.T) {
		code := `
class Box {
    cmd: string = "";
    safe: string = "";
}

function run(holder: Box) {
    console.log(holder.cmd);
}

function assign(cmd: string) {
    const value = new Box();
    value.cmd = cmd;
    value.safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `console.log(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-cmd"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("negative safe field", func(t *testing.T) {
		code := `
class Box {
    cmd: string = "";
    safe: string = "";
}

function run(holder: Box) {
    console.log(holder.safe);
}

function assign(cmd: string) {
    const value = new Box();
    value.cmd = cmd;
    value.safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `console.log(* #-> * as $target)`, map[string][]string{
			"target": {`"safe"`},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})
}
