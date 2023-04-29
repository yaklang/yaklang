package yakast

import "yaklang/common/yak/antlr4yak/yakvm"

func init() {
	yakvm.YakDebugCompiler = NewYakCompiler()
}
