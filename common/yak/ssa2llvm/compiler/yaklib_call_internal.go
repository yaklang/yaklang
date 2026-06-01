package compiler

import (
	"fmt"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// Ensure yaklang stdlib modules are visible during compile-time dispatch resolution.
import _ "github.com/yaklang/yaklang/common/yak"

func splitQualifiedName(name string) (pkg, method string, ok bool) {
	name = strings.TrimSpace(name)
	idx := strings.LastIndex(name, ".")
	if idx <= 0 || idx >= len(name)-1 {
		return "", "", false
	}
	return name[:idx], name[idx+1:], true
}

func (c *Compiler) shouldUseYaklibDispatch(calleeName string) bool {
	if _, ok := c.getExternBinding(calleeName); ok {
		return false
	}
	if _, ok := yaklang.LookupGlobalCallable(calleeName); ok {
		return true
	}
	pkg, method, ok := splitQualifiedName(calleeName)
	if !ok || method == "" {
		return false
	}
	if _, ok := yaklang.LookupExport(pkg, method); ok {
		return true
	}
	// Some stdlib tables are only visible via the interpreter Fntable map.
	if table, ok := yaklang.New().GetFntable()[pkg]; ok {
		if exports, ok := table.(map[string]any); ok {
			_, ok := exports[method]
			return ok
		}
	}
	return false
}

func (c *Compiler) newYaklibDispatchSpec(inst *ssa.Call, pkg, method string) (contextCallSpec, error) {
	if inst == nil {
		return contextCallSpec{}, fmt.Errorf("newYaklibDispatchSpec: missing call instruction")
	}
	i64 := c.LLVMCtx.Int64Type()
	pkgPtr := c.Builder.CreateGlobalStringPtr(pkg, fmt.Sprintf("yaklib_pkg_%d", inst.GetId()))
	methodPtr := c.Builder.CreateGlobalStringPtr(method, fmt.Sprintf("yaklib_method_%d", inst.GetId()))
	args := make([]contextCallArg, 0, len(inst.Args)+2)
	args = append(args,
		contextCallArg{value: llvm.ConstPtrToInt(pkgPtr, i64), tagPointerArg: true},
		contextCallArg{value: llvm.ConstPtrToInt(methodPtr, i64), tagPointerArg: true},
	)
	for _, argID := range inst.Args {
		args = append(args, contextCallArg{ssaID: argID, tagPointerArg: true})
	}
	return contextCallSpec{
		inst:      inst,
		kind:      abi.KindDispatch,
		target:    llvm.ConstInt(i64, uint64(abi.IDYaklibCall), false),
		args:      args,
		async:     inst.Async,
		ctxName:   "yak_yaklib_ctx",
		errPrefix: "emitYaklibDispatch",
	}, nil
}

func (c *Compiler) newRuntimeInDispatchSpec(inst *ssa.BinOp) (contextCallSpec, error) {
	if inst == nil {
		return contextCallSpec{}, fmt.Errorf("newRuntimeInDispatchSpec: missing binop instruction")
	}
	args := []contextCallArg{
		{ssaID: inst.X, tagPointerArg: true},
		{ssaID: inst.Y, tagPointerArg: true},
	}
	return contextCallSpec{
		inst:      inst,
		kind:      abi.KindDispatch,
		target:    llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(abi.IDRuntimeIn), false),
		args:      args,
		async:     false,
		ctxName:   "yak_in_ctx",
		errPrefix: "emitRuntimeInDispatch",
	}, nil
}
