package javascript

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJSFieldSensitiveTypeFallbackWithoutCallGraph(t *testing.T) {
	t.Run("positive cmd field", func(t *testing.T) {
		code := `
class Box {
    constructor() {
        this.cmd = null;
        this.safe = null;
    }
}

function run(holder) {
    console.log(holder.cmd);
}

function assign(cmd) {
    const holder = new Box();
    holder.cmd = cmd;
    holder.safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `console.log(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-cmd"},
		}, ssaapi.WithLanguage(ssaconfig.JS))
	})

	t.Run("negative safe field", func(t *testing.T) {
		code := `
class Box {
    constructor() {
        this.cmd = null;
        this.safe = null;
    }
}

function run(holder) {
    console.log(holder.safe);
}

function assign(cmd) {
    const holder = new Box();
    holder.cmd = cmd;
    holder.safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `console.log(* #-> * as $target)`, map[string][]string{
			"target": {`"safe"`},
		}, ssaapi.WithLanguage(ssaconfig.JS))
	})
}
