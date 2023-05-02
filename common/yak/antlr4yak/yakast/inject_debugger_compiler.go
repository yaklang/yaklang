package yakast

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

func init() {
	yakvm.YakDebugCompiler = NewYakCompiler()
}
