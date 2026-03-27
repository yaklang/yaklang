package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) ctxI64Ptr() (llvm.Value, error) {
	if c == nil || c.function == nil || c.function.invokeCtx.IsNil() {
		return llvm.Value{}, fmt.Errorf("missing invoke context")
	}
	i64Ptr := llvm.PointerType(c.LLVMCtx.Int64Type(), 0)
	if c.function.invokeCtx.Type() == i64Ptr {
		return c.function.invokeCtx, nil
	}
	return c.Builder.CreateBitCast(c.function.invokeCtx, i64Ptr, "yak_ctx_i64p"), nil
}

func (c *Compiler) ctxWordPtr(word int64) (llvm.Value, error) {
	ctxPtr, err := c.ctxI64Ptr()
	if err != nil {
		return llvm.Value{}, err
	}
	i64 := c.LLVMCtx.Int64Type()
	idx := llvm.ConstInt(i64, uint64(word), false)
	return c.Builder.CreateGEP(i64, ctxPtr, []llvm.Value{idx}, ""), nil
}

func (c *Compiler) loadContextWord(word int64, name string) (llvm.Value, error) {
	ptr, err := c.ctxWordPtr(word)
	if err != nil {
		return llvm.Value{}, err
	}
	i64 := c.LLVMCtx.Int64Type()
	return c.Builder.CreateLoad(i64, ptr, name), nil
}

func (c *Compiler) storeContextWord(word int64, val llvm.Value) error {
	ptr, err := c.ctxWordPtr(word)
	if err != nil {
		return err
	}
	c.Builder.CreateStore(c.coerceToInt64(val), ptr)
	return nil
}

func (c *Compiler) storeContextReturn(val llvm.Value) error {
	return c.storeContextWord(abi.WordRet, val)
}

func (c *Compiler) getOrInsertRuntimeLoadPanicValue() (llvm.Value, llvm.Type) {
	name := "yak_runtime_load_panic_value"
	fn := c.Mod.NamedFunction(name)
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.Int64Type(), []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) clearContextFlags(mask uint64) error {
	flags, err := c.loadContextWord(abi.WordFlags, "yak_ctx_flags")
	if err != nil {
		return err
	}
	cleared := c.Builder.CreateAnd(flags, llvm.ConstInt(c.LLVMCtx.Int64Type(), ^mask, false), "yak_ctx_flags_clear")
	return c.storeContextWord(abi.WordFlags, cleared)
}

func (c *Compiler) setContextFlags(mask uint64) error {
	flags, err := c.loadContextWord(abi.WordFlags, "yak_ctx_flags")
	if err != nil {
		return err
	}
	set := c.Builder.CreateOr(flags, llvm.ConstInt(c.LLVMCtx.Int64Type(), mask, false), "yak_ctx_flags_set")
	return c.storeContextWord(abi.WordFlags, set)
}

func (c *Compiler) storeContextPanic(val llvm.Value, flags uint64) error {
	if err := c.storeContextWord(abi.WordPanic, val); err != nil {
		return err
	}
	if err := c.clearContextFlags(abi.FlagPanicTaggedPointer); err != nil {
		return err
	}
	if flags != 0 {
		return c.setContextFlags(flags)
	}
	return nil
}

func (c *Compiler) loadContextPanic(name string) (llvm.Value, error) {
	if c == nil || c.function == nil || c.function.invokeCtx.IsNil() {
		return llvm.Value{}, fmt.Errorf("missing invoke context")
	}
	loadFn, loadType := c.getOrInsertRuntimeLoadPanicValue()
	return c.Builder.CreateCall(loadType, loadFn, []llvm.Value{c.function.invokeCtx}, name), nil
}

func (c *Compiler) bindParamsFromContext(fn *ssa.Function) error {
	if fn == nil {
		return nil
	}

	ctxPtr, err := c.ctxI64Ptr()
	if err != nil {
		return err
	}

	i64 := c.LLVMCtx.Int64Type()
	argBase := int64(abi.HeaderWords)

	for i, paramID := range fn.Params {
		idx := llvm.ConstInt(i64, uint64(argBase+int64(i)), false)
		elemPtr := c.Builder.CreateGEP(i64, ctxPtr, []llvm.Value{idx}, "")
		val := c.Builder.CreateLoad(i64, elemPtr, fmt.Sprintf("arg_%d", paramID))
		c.Values[paramID] = val
	}

	for i, memberID := range fn.ParameterMembers {
		paramIndex := int64(len(fn.Params) + i)
		idx := llvm.ConstInt(i64, uint64(argBase+paramIndex), false)
		elemPtr := c.Builder.CreateGEP(i64, ctxPtr, []llvm.Value{idx}, "")
		val := c.Builder.CreateLoad(i64, elemPtr, fmt.Sprintf("pm_%d", memberID))
		c.Values[memberID] = val
	}

	freeValueBase := int64(len(fn.Params) + len(fn.ParameterMembers))
	for i, binding := range orderedFreeValueBindings(fn) {
		idx := llvm.ConstInt(i64, uint64(argBase+freeValueBase+int64(i)), false)
		elemPtr := c.Builder.CreateGEP(i64, ctxPtr, []llvm.Value{idx}, "")
		val := c.Builder.CreateLoad(i64, elemPtr, fmt.Sprintf("fv_%d", binding.ssaID))
		c.Values[binding.ssaID] = val
	}

	return nil
}
