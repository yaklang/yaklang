package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type NativeCallActualParams struct {
	m map[string]any
}

func NewNativeCallActualParams(items ...*RecursiveConfigItem) *NativeCallActualParams {
	n := &NativeCallActualParams{
		m: make(map[string]any),
	}
	for _, i := range items {
		n.m[i.Key] = i.Value
	}
	return n
}

func (n *NativeCallActualParams) Existed(index any) bool {
	_, ok := n.m[codec.AnyToString(index)]
	return ok
}

func (n *NativeCallActualParams) GetString(index any) string {
	raw, ok := n.m[codec.AnyToString(index)]
	if ok {
		return codec.AnyToString(raw)
	}
	return ""
}

func (n *NativeCallActualParams) GetInt(index any) int {
	raw, ok := n.m[codec.AnyToString(index)]
	if ok {
		return codec.Atoi(codec.AnyToString(raw))
	}
	return 0
}

type NativeCallFunc func(v ValueOperator, frame *SFFrame, params *NativeCallActualParams) (bool, ValueOperator, error)

var nativeCallTable = map[string]NativeCallFunc{}

func RegisterNativeCall(name string, f NativeCallFunc) {
	nativeCallTable[name] = f
}

func GetNativeCall(name string) (NativeCallFunc, error) {
	if f, ok := nativeCallTable[name]; ok {
		return f, nil
	}
	return nil, utils.Wrap(CriticalError, "native call not found: "+name)
}
