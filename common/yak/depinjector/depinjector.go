package depinjector

import (
	// import aiengine to register yak.AIEngineExports via init()
	_ "github.com/yaklang/yaklang/common/aiengine"

	// import yakgrpc to register mcp.NewLocalClient via init()
	_ "github.com/yaklang/yaklang/common/yakgrpc"
)

// DependencyInject is kept for backward compatibility
// All registrations are now done via init() functions in the respective packages:
// - aiengine: registers yak.AIEngineExports
// - yakgrpc: registers mcp.NewLocalClient
func DependencyInject() {
	// All registrations are now automatic via init()
}
