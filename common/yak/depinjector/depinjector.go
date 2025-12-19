package depinjector

import (
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/yak"

	// import yakgrpc to register mcp.NewLocalClient via init()
	_ "github.com/yaklang/yaklang/common/yakgrpc"
)

func DependencyInject() {
	yak.AIEngineExports = aiengine.Exports
}
