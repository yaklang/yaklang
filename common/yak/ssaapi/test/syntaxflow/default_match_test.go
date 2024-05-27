package syntaxflow

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func TestDefaultMatch(t *testing.T) {
	prog, _ := ssaapi.Parse(`
a = b => {
	return b + 4
}

dump(a(2))

`)
	prog.SyntaxFlowChain(`dump(* #-> *)`, sfvm.WithEnableDebug())
}
