package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) allocInvokeContext(argc int, name string) (llvm.Value, llvm.Value, error) {
	if c == nil {
		return llvm.Value{}, llvm.Value{}, fmt.Errorf("allocInvokeContext: compiler is nil")
	}
	if argc < 0 {
		return llvm.Value{}, llvm.Value{}, fmt.Errorf("allocInvokeContext: invalid argc %d", argc)
	}

	i64 := c.LLVMCtx.Int64Type()
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	i64Ptr := llvm.PointerType(i64, 0)

	words := int64(abi.HeaderWords + argc + argc)
	sizeBytes := llvm.ConstInt(i64, uint64(words*8), false)

	mallocFn, mallocType := c.getOrInsertMalloc()
	rawPtr := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{sizeBytes}, name+"_mem")

	ctxI8 := c.Builder.CreateIntToPtr(rawPtr, i8Ptr, name)
	ctxI64 := c.Builder.CreateIntToPtr(rawPtr, i64Ptr, name+"_i64p")
	return ctxI8, ctxI64, nil
}

func (c *Compiler) ctxWordPtrFrom(ctxI64 llvm.Value, word int64) (llvm.Value, error) {
	if c == nil {
		return llvm.Value{}, fmt.Errorf("ctxWordPtrFrom: compiler is nil")
	}
	if ctxI64.IsNil() {
		return llvm.Value{}, fmt.Errorf("ctxWordPtrFrom: ctx is nil")
	}
	i64 := c.LLVMCtx.Int64Type()
	idx := llvm.ConstInt(i64, uint64(word), false)
	return c.Builder.CreateGEP(i64, ctxI64, []llvm.Value{idx}, ""), nil
}

func (c *Compiler) storeCtxWordFrom(ctxI64 llvm.Value, word int64, val llvm.Value) error {
	ptr, err := c.ctxWordPtrFrom(ctxI64, word)
	if err != nil {
		return err
	}
	c.Builder.CreateStore(c.coerceToInt64(val), ptr)
	return nil
}

func (c *Compiler) loadCtxWordFrom(ctxI64 llvm.Value, word int64, name string) (llvm.Value, error) {
	ptr, err := c.ctxWordPtrFrom(ctxI64, word)
	if err != nil {
		return llvm.Value{}, err
	}
	i64 := c.LLVMCtx.Int64Type()
	return c.Builder.CreateLoad(i64, ptr, name), nil
}

func (c *Compiler) initInvokeContext(ctxI64 llvm.Value, kind uint64, target llvm.Value, argc int) error {
	if c == nil {
		return fmt.Errorf("initInvokeContext: compiler is nil")
	}
	if argc < 0 {
		return fmt.Errorf("initInvokeContext: invalid argc %d", argc)
	}
	i64 := c.LLVMCtx.Int64Type()

	if err := c.storeCtxWordFrom(ctxI64, abi.WordMagic, llvm.ConstInt(i64, abi.Magic, false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordVersion, llvm.ConstInt(i64, abi.Version, false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordKind, llvm.ConstInt(i64, kind, false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordFlags, llvm.ConstInt(i64, 0, false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordTarget, target); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordArgc, llvm.ConstInt(i64, uint64(argc), false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordRet, llvm.ConstInt(i64, 0, false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordPanic, llvm.ConstInt(i64, 0, false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordReserved, llvm.ConstInt(i64, 0, false)); err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordReserved+1, llvm.ConstInt(i64, 0, false)); err != nil {
		return err
	}
	return nil
}

func (c *Compiler) storeInvokeContextArg(ctxI64 llvm.Value, index int, val llvm.Value) error {
	word := int64(abi.HeaderWords) + int64(index)
	return c.storeCtxWordFrom(ctxI64, word, val)
}

func (c *Compiler) storeInvokeContextRoot(ctxI64 llvm.Value, argc int, index int, val llvm.Value) error {
	word := int64(abi.HeaderWords) + int64(argc) + int64(index)
	return c.storeCtxWordFrom(ctxI64, word, val)
}
