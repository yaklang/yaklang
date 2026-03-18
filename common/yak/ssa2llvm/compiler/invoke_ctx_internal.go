package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) ctxI64Ptr() (llvm.Value, error) {
	if c == nil || c.invokeCtx.IsNil() {
		return llvm.Value{}, fmt.Errorf("missing invoke context")
	}
	i64Ptr := llvm.PointerType(c.LLVMCtx.Int64Type(), 0)
	if c.invokeCtx.Type() == i64Ptr {
		return c.invokeCtx, nil
	}
	return c.Builder.CreateBitCast(c.invokeCtx, i64Ptr, "yak_ctx_i64p"), nil
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

func (c *Compiler) storeContextPanic(val llvm.Value) error {
	return c.storeContextWord(abi.WordPanic, val)
}

func (c *Compiler) loadContextPanic(name string) (llvm.Value, error) {
	return c.loadContextWord(abi.WordPanic, name)
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

	return nil
}
