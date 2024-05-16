package information

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var (
	AI_PLUGIN           = "AI插件"
	DROP_HTTP_PACKET    = "可能丢弃HTTP数据包"
	FORWARD_HTTP_PACKET = "可能修改HTTP数据包"
)

func ParseTags(prog *ssaapi.Program) []string {
	ret := make([]string, 0)
	// check ai
	valueMap, err := prog.SyntaxFlowWithError("ai.Chat() as $target")
	if err == nil {
		if vs, ok := valueMap["target"]; ok && vs.Len() > 0 {
			ret = append(ret, AI_PLUGIN)
		}
	}
	checkParamHasCall := func(ref ssaapi.Values, index int, tag string) {
		funcDel := GetLastRef(ref)
		if !funcDel.IsNil() && funcDel.IsFunction() {
			if funcDel.GetParameter(index).GetCalledBy().Len() > 0 {
				ret = append(ret, tag)
			}
		}
	}

	// check mitm hijack
	hijackHTTPRequest := prog.Ref("hijackHTTPRequest")
	hijackHTTPResponse := prog.Ref("hijackHTTPResponse")
	hijackHTTPResponseEx := prog.Ref("hijackHTTPResponseEx")
	checkParamHasCall(hijackHTTPRequest, 3, FORWARD_HTTP_PACKET)
	checkParamHasCall(hijackHTTPRequest, 4, DROP_HTTP_PACKET)
	checkParamHasCall(hijackHTTPResponse, 3, FORWARD_HTTP_PACKET)
	checkParamHasCall(hijackHTTPResponse, 4, DROP_HTTP_PACKET)
	checkParamHasCall(hijackHTTPResponseEx, 4, FORWARD_HTTP_PACKET)
	checkParamHasCall(hijackHTTPResponseEx, 5, DROP_HTTP_PACKET)
	return ret
}

func GetLastRef(vs ssaapi.Values) *ssaapi.Value {
	var ret *ssaapi.Value
	vs.ForEach(func(v *ssaapi.Value) {
		if ret == nil {
			ret = v
		}
		if v.GetRange().GetOffset() > ret.GetRange().GetOffset() {
			ret = v
		}
	})
	return ret
}
