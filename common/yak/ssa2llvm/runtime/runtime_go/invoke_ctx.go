package main

import (
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func ctxLoadWord(ctx unsafe.Pointer, word int) uint64 {
	return *(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(word)*8))
}

func ctxStoreWord(ctx unsafe.Pointer, word int, value uint64) {
	*(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(word)*8)) = value
}

func ctxFlags(ctx unsafe.Pointer) uint64 {
	return ctxLoadWord(ctx, abi.WordFlags)
}

func ctxStoreFlags(ctx unsafe.Pointer, flags uint64) {
	ctxStoreWord(ctx, abi.WordFlags, flags)
}

func ctxSetFlags(ctx unsafe.Pointer, flags uint64) {
	ctxStoreFlags(ctx, ctxFlags(ctx)|flags)
}

func ctxClearFlags(ctx unsafe.Pointer, flags uint64) {
	ctxStoreFlags(ctx, ctxFlags(ctx)&^flags)
}

func ctxArgc(ctx unsafe.Pointer) int {
	return int(int64(ctxLoadWord(ctx, abi.WordArgc)))
}

func ctxArgsSlice(ctx unsafe.Pointer, argc int) []uint64 {
	if argc <= 0 || ctx == nil {
		return nil
	}
	base := (*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(abi.HeaderWords)*8))
	return unsafe.Slice(base, argc)
}

func ctxRootsSlice(ctx unsafe.Pointer, argc int) []uint64 {
	if argc <= 0 || ctx == nil {
		return nil
	}
	start := abi.HeaderWords + argc
	base := (*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(start)*8))
	return unsafe.Slice(base, argc)
}

func ctxInit(ctx unsafe.Pointer, kind uint64, target uint64, argc int) {
	if ctx == nil {
		return
	}
	ctxStoreWord(ctx, abi.WordMagic, abi.Magic)
	ctxStoreWord(ctx, abi.WordVersion, abi.Version)
	ctxStoreWord(ctx, abi.WordKind, kind)
	ctxStoreWord(ctx, abi.WordFlags, 0)
	ctxStoreWord(ctx, abi.WordTarget, target)
	ctxStoreWord(ctx, abi.WordArgc, uint64(argc))
	ctxStoreWord(ctx, abi.WordRet, 0)
	ctxStoreWord(ctx, abi.WordPanic, 0)
	ctxStoreWord(ctx, abi.WordReserved, 0)
	ctxStoreWord(ctx, abi.WordReserved+1, 0)
}

func ctxSetRet(ctx unsafe.Pointer, value int64) {
	if ctx == nil {
		return
	}
	ctxStoreWord(ctx, abi.WordRet, uint64(value))
}

func ctxSetPanic(ctx unsafe.Pointer, value uint64, flags uint64) {
	if ctx == nil {
		return
	}
	ctxStoreWord(ctx, abi.WordPanic, value)
	ctxClearFlags(ctx, abi.FlagPanicTaggedPointer)
	if flags != 0 {
		ctxSetFlags(ctx, flags)
	}
}

func ctxNormalizedPanicValue(ctx unsafe.Pointer) int64 {
	if ctx == nil {
		return 0
	}
	value := ctxLoadWord(ctx, abi.WordPanic)
	if (ctxFlags(ctx) & abi.FlagPanicTaggedPointer) != 0 {
		value |= yakTaggedPointerMask
	}
	return int64(value)
}
