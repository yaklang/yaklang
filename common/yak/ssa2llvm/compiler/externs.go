package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

type LLVMExternType uint8

const (
	ExternTypeVoid LLVMExternType = iota
	ExternTypeI64
	ExternTypePtr
)

type ExternBinding struct {
	// Symbol is the target runtime symbol for a direct extern call.
	// When DispatchID is non-zero, Symbol/Params are ignored and the call
	// is routed through the stdlib dispatcher.
	Symbol string
	Params []LLVMExternType
	Return LLVMExternType

	// DispatchID identifies a stdlib function that should be invoked via the
	// runtime dispatcher entry (yak_std_call). Keep it opaque to reduce the
	// number of exported symbols in the final binary.
	DispatchID abi.FuncID
}

var defaultExternBindings = map[string]ExternBinding{
	"println": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDPrintln,
	},
	"print": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDPrint,
	},
	"printf": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDPrintf,
	},
	"append": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDAppend,
	},
	"yakit.Info": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDYakitInfo,
	},
	"yakit.Warn": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDYakitWarn,
	},
	"yakit.Debug": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDYakitDebug,
	},
	"yakit.Error": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDYakitError,
	},
	"sync.NewWaitGroup": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewWaitGroup,
	},
	"sync.NewSizedWaitGroup": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewSizedWaitGroup,
	},
	"sync.NewLock": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewLock,
	},
	"sync.NewMutex": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewMutex,
	},
	"sync.NewRWMutex": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewRWMutex,
	},
	"sync.NewMap": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewMap,
	},
	"sync.NewOnce": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewOnce,
	},
	"sync.NewPool": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewPool,
	},
	"sync.NewCond": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDSyncNewCond,
	},
	"poc.timeout": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDPocTimeout,
	},
	"poc.Get": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDPocGet,
	},
	"poc.GetHTTPPacketBody": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDPocGetHTTPPacketBody,
	},
	"os.Getenv": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDOsGetenv,
	},
}

func cloneExternBindings(src map[string]ExternBinding) map[string]ExternBinding {
	out := make(map[string]ExternBinding, len(src))
	for k, v := range src {
		params := make([]LLVMExternType, len(v.Params))
		copy(params, v.Params)
		out[k] = ExternBinding{
			Symbol:     v.Symbol,
			Params:     params,
			Return:     v.Return,
			DispatchID: v.DispatchID,
		}
	}
	return out
}

func mergeExternBindings(base map[string]ExternBinding, custom map[string]ExternBinding) map[string]ExternBinding {
	out := cloneExternBindings(base)
	for k, v := range custom {
		params := make([]LLVMExternType, len(v.Params))
		copy(params, v.Params)
		out[k] = ExternBinding{
			Symbol:     v.Symbol,
			Params:     params,
			Return:     v.Return,
			DispatchID: v.DispatchID,
		}
	}
	return out
}

func (c *Compiler) getExternBinding(name string) (ExternBinding, bool) {
	if c == nil || c.ExternBindings == nil {
		return ExternBinding{}, false
	}
	b, ok := c.ExternBindings[name]
	return b, ok
}

func (c *Compiler) llvmTypeForExtern(t LLVMExternType) llvm.Type {
	switch t {
	case ExternTypeVoid:
		return c.LLVMCtx.VoidType()
	case ExternTypePtr:
		return llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	case ExternTypeI64:
		fallthrough
	default:
		return c.LLVMCtx.Int64Type()
	}
}

func (c *Compiler) ensureExternDeclaration(name string) llvm.Value {
	binding, ok := c.getExternBinding(name)
	if !ok {
		return llvm.Value{}
	}

	fn := c.Mod.NamedFunction(binding.Symbol)
	if !fn.IsNil() {
		return fn
	}

	params := make([]llvm.Type, 0, len(binding.Params))
	for _, p := range binding.Params {
		params = append(params, c.llvmTypeForExtern(p))
	}
	retType := c.llvmTypeForExtern(binding.Return)
	fnType := llvm.FunctionType(retType, params, false)
	fn = llvm.AddFunction(c.Mod, binding.Symbol, fnType)
	return fn
}
