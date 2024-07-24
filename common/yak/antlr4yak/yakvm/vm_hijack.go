package yakvm

import "github.com/yaklang/yaklang/common/utils"

func (v *Frame) execHijackMapMemberCallHandler(caller, callee string, origin interface{}) interface{} {
	val, ok := v.hijackMapMemberCallHandlers.Load(utils.CalcSha1(caller, callee))
	if !ok {
		return origin
	} else {
		return val.(func(interface{}) interface{})(origin)
	}
}
