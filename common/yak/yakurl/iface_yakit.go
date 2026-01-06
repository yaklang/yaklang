//go:build !yakit_exclude

package yakurl

import (
	"github.com/yaklang/yaklang/common/wsm"
	"github.com/yaklang/yaklang/common/yak/yakurl/java_decompiler"
)

// createYakitAction 创建 Yakit 专用的 action
// 这个函数只在非 yakit_exclude 模式下可用
func createYakitAction(schema string) Action {
	switch schema {
	case "website":
		return &websiteFromHttpFlow{}
	case "behinder":
		return &wsm.BehidnerResourceSystemAction{}
	case "godzilla":
		return &wsm.GodzillaFileSystemAction{}
	case "fuzztag":
		return &fuzzTagDocAction{}
	case "yakdocument":
		return &documentAction{}
	case "facades":
		return newFacadeServerAction()
	case "yakshell":
		return &wsm.YakShellResourceAction{}
	case "javadec":
		return java_decompiler.NewJavaDecompilerAction()
	default:
		return nil
	}
}
