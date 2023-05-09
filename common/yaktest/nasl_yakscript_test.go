package yaktest

import (
	"github.com/yaklang/yaklang/common/yak"
	"testing"
)

func TestName(t *testing.T) {
	engine := yak.NewScriptEngine(1)
	engine.Execute(`
nasl.UpdateDatabase("/Users/z3/nasl/nasl-plugins/2023/apache")
`)
}
