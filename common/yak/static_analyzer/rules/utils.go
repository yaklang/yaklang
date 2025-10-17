package rules

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var (
	Number        = ssaapi.NewType(ssa.CreateNumberType())
	String        = ssaapi.NewType(ssa.CreateStringType())
	Bytes         = ssaapi.NewType(ssa.CreateBytesType())
	Boolean       = ssaapi.NewType(ssa.CreateBooleanType())
	UndefinedType = ssaapi.NewType(ssa.CreateUndefinedType())
	Null          = ssaapi.NewType(ssa.CreateNullType())
	Any           = ssaapi.NewType(ssa.CreateAnyType())
	ErrorType     = ssaapi.NewType(ssa.CreateErrorType())
)
