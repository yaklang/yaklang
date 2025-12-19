package yak

import (
	"github.com/yaklang/yaklang/common/utils"
)

// AIEngineExports is the exports map for "aim" module
// It should be registered via RegisterAIEngineExports by importing the aiengine package
var AIEngineExports map[string]interface{}

func init() {
	// Initialize with a default error-returning stub
	// This will be replaced when aiengine package is imported
	AIEngineExports = map[string]interface{}{
		"InvokeReAct": func(args ...interface{}) (interface{}, error) {
			return nil, utils.Error("aiengine not registered, please import 'github.com/yaklang/yaklang/common/aiengine'")
		},
	}
}

// RegisterAIEngineExports registers the aiengine exports
// This should be called by the aiengine package's init function
func RegisterAIEngineExports(exports map[string]interface{}) {
	if exports != nil {
		AIEngineExports = exports
	}
}
