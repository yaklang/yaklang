package yakvm

import "yaklang/common/utils"

func (v *Frame) hijackMapMemberCall(caller, callee string, hijacker func(i interface{}) interface{}) {
	v.hijackMapMemberCallHandlers.Store(utils.CalcSha1(caller, callee), hijacker)
}

func (v *Frame) execHijackMapMemberCallHandler(caller, callee string, origin interface{}) interface{} {
	val, ok := v.hijackMapMemberCallHandlers.Load(utils.CalcSha1(caller, callee))
	if !ok {
		return origin
	} else {
		return val.(func(interface{}) interface{})(origin)
	}
}
