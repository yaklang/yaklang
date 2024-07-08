package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
)

type NativeCallFunc func(v ValueOperator, frame *SFFrame) (bool, ValueOperator, error)

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
