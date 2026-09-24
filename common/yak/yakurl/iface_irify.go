//go:build !irify_exclude

package yakurl

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// createIrifyAction 创建 Irify 专用的 action
// 这个函数只在非 irify_exclude 模式下可用
func createIrifyAction(schema string) Action {
	switch schema {
	case "syntaxflow":
		return NewSyntaxFlowAction()
	case "ssadb":
		return &fileSystemAction{
			fs: ssaapi.NewProgramFileSystem(),
		}
	case "ssarisk":
		return &riskTreeAction{
			register: make(map[string]int),
		}
	default:
		return nil
	}
}
