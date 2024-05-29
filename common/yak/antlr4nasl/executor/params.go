package executor

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

type NaslBuildInMethodParam struct {
	MapParams  map[string]*yakvm.Value
	ListParams []*yakvm.Value
}

func NewNaslBuildInMethodParam() *NaslBuildInMethodParam {
	return &NaslBuildInMethodParam{MapParams: make(map[string]*yakvm.Value)}
}

var empty = yakvm.NewValue("empty", nil, "empty")

func (n *NaslBuildInMethodParam) GetParamByNumber(index int, defaultValue ...interface{}) *yakvm.Value {
	if index < len(n.ListParams) {
		return n.ListParams[index]
	} else {
		if len(defaultValue) != 0 {
			return yakvm.NewAutoValue(defaultValue[0])
		}
		return empty
	}
}
func (n *NaslBuildInMethodParam) GetParamByName(name string, defaultValue ...interface{}) *yakvm.Value {
	if v, ok := n.MapParams[name]; ok {
		return v
	} else {
		if len(defaultValue) != 0 {
			return yakvm.NewAutoValue(defaultValue[0])
		}
		return yakvm.GetUndefined()
	}
}
func ForEachParams(params *NaslBuildInMethodParam, handle func(value *yakvm.Value)) {
	var item *yakvm.Value
	for i := 0; ; i++ {
		item = params.GetParamByNumber(i)
		if item == empty {
			break
		}
		handle(item)
	}
}
