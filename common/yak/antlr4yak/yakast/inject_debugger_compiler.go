package yakast

import "yaklang.io/yaklang/common/yak/antlr4yak/yakvm"

func init() {
	yakvm.YakDebugCompiler = NewYakCompiler()
}
