package java2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	if ssaconfig.BuildCompileExcludeFunc(nil, "")(path) {
		return false
	}
	return ssaconfig.MatchJavaPreHandlerFile(path)
}
