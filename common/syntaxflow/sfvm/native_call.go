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

func (n *NativeCallActualParams) GetString(index any, extra ...any) string {
	if n == nil {
		return ""
	}

	raw, ok := n.m[codec.AnyToString(index)]
	if ok {
		return codec.AnyToString(raw)
	}

	for _, name := range extra {
		raw, ok = n.m[codec.AnyToString(name)]
		if ok {
			return codec.AnyToString(raw)
		}
	}

	return ""
}

func (n *NativeCallActualParams) GetInt(index any, extra ...any) int {
	if n == nil {
		return -1
	}
	raw, ok := n.m[codec.AnyToString(index)]
	if ok {
		return codec.Atoi(codec.AnyToString(raw))
	}

	for _, name := range extra {
		raw, ok := n.m[codec.AnyToString(name)]
		if ok {
			return codec.Atoi(codec.AnyToString(raw))
		}
	}
	return -1
}

type NativeCallFunc func(v ValueOperator, frame *SFFrame, params *NativeCallActualParams) (bool, ValueOperator, error)

type NativeCall struct {
	Name        string
	Description string
	Function    NativeCallFunc
}

func NewNativeCall(name string, description string, function NativeCallFunc) *NativeCall {
	return &NativeCall{
		Name:        name,
		Description: description,
		Function:    function,
	}
}

var nativeCallTable = map[string]*NativeCall{}

func RegisterNativeCall(name string, description string, function NativeCallFunc) {
	nativeCallTable[name] = NewNativeCall(name, description, function)
}

func GetNativeCall(name string) (NativeCallFunc, string, error) {
	call, ok := nativeCallTable[name]
	if !ok {
		return nil, "", utils.Errorf("native call not found: %s", name)
	}
	return call.Function, call.Description, nil
}
