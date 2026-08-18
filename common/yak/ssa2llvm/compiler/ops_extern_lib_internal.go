package compiler

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func splitExternValueName(name string) (pkg, key string, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (c *Compiler) compileYaklibExportMember(contextInst ssa.Instruction, val ssa.Value, pkg, keyStr string) error {
	if val == nil || pkg == "" || keyStr == "" {
		return nil
	}
	exported, ok := yaklang.LookupExport(pkg, keyStr)
	if !ok || exported == nil {
		return nil
	}
	rv := reflect.ValueOf(exported)
	if rv.IsValid() && rv.Kind() == reflect.Func {
		return nil
	}
	if rv.IsValid() && rv.Kind() == reflect.Func {
		c.recordYaklibDependency(pkg, keyStr)
		c.cacheValue(val.GetId(), c.materializeYaklibExportCallable(val, pkg, keyStr))
		return c.maybeEmitMemberSet(contextInst, val, val.GetId())
	}
	// String exports (e.g. ssa.GO, a named string Language constant) must be
	// lowered as a global C-string pointer, mirroring how string literals are
	// emitted. Boxing them through runtimeValueToInt64ForCompiler would return
	// 0 (empty string) because that helper only handles numeric/bool values.
	if rv.IsValid() && rv.Kind() == reflect.String {
		ptr := c.Builder.CreateGlobalStringPtr(rv.String(), fmt.Sprintf("yaklib_export_str_%d", val.GetId()))
		// The OR instruction must dominate every use (member-set syncs can run in
		// other blocks), so anchor it at the value's definition point.
		var tagged llvm.Value
		c.withSSADefInsertPoint(val.GetId(), func() {
			tagged = c.Builder.CreateOr(llvm.ConstPtrToInt(ptr, c.LLVMCtx.Int64Type()), llvm.ConstInt(c.LLVMCtx.Int64Type(), yakTaggedPointerMask, false), "yaklib_export_str_tag")
		})
		c.cacheValue(val.GetId(), tagged)
		return c.maybeEmitMemberSet(contextInst, val, val.GetId())
	}
	boxed := runtimeValueToInt64ForCompiler(exported)
	c.cacheValue(val.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(boxed), false))
	return c.maybeEmitMemberSet(contextInst, val, val.GetId())
}

func (c *Compiler) compileExternLibMember(
	contextInst ssa.Instruction,
	val ssa.Value,
	extern *ssa.ExternLib,
	key ssa.Value,
	keyStr string,
) error {
	if val == nil || extern == nil {
		return fmt.Errorf("compileExternLibMember: missing value or extern lib")
	}

	memberID := val.GetId()
	if memberID <= 0 {
		return nil
	}

	if _, ok := c.getCachedValue(contextInst, memberID); ok {
		return nil
	}

	pkg := extern.LibraryName
	if pkg == "" {
		pkg = extern.GetName()
	}
	if err := c.compileYaklibExportMember(contextInst, val, pkg, keyStr); err != nil {
		return err
	}
	if _, ok := c.getCachedValue(contextInst, memberID); ok {
		return nil
	}

	// Pair-first SSA: extern members are stored as member pairs on the value.
	// Resolve by exact key string (e.g. "PoCExports") or by key Value.
	var memberValID int64
	if keyStr != "" {
		members := extern.GetMembersByKeyString(keyStr)
		if len(members) > 0 && !utils.IsNil(members[0]) {
			memberValID = members[0].GetId()
		}
	}
	if memberValID == 0 && key != nil {
		members := extern.GetMembersByExactKey(key)
		if len(members) > 0 && !utils.IsNil(members[0]) {
			memberValID = members[0].GetId()
		}
	}

	if memberValID != 0 {
		if memberValID == memberID {
			zero := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
			c.cacheValue(memberID, zero)
			return c.maybeEmitMemberSet(contextInst, val, memberID)
		}
		if memberVal, ok := contextInst.GetFunc().GetValueById(memberValID); ok {
			if undef, ok := ssa.ToUndefined(memberVal); ok && undef != nil && undef.IsExtern() {
				if err := c.compileYaklibExportMember(contextInst, val, pkg, keyStr); err != nil {
					return err
				}
				if _, ok := c.getCachedValue(contextInst, memberID); ok {
					return c.maybeEmitMemberSet(contextInst, val, memberID)
				}
			}
		}
		memberLLVM, err := c.getValue(contextInst, memberValID)
		if err != nil {
			return fmt.Errorf("compileExternLibMember: resolve member %q: %w", keyStr, err)
		}
		c.cacheValue(memberID, c.coerceToInt64(memberLLVM))
		return c.maybeEmitMemberSet(contextInst, val, memberID)
	}

	zero := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	c.cacheValue(memberID, zero)
	return c.maybeEmitMemberSet(contextInst, val, memberID)
}

// runtimeValueToInt64ForCompiler mirrors the runtime boxing rules without importing
// the c-archive runtime package (compiler is a separate Go target).
func runtimeValueToInt64ForCompiler(v any) int64 {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return 0
	}
	if rv.CanInt() {
		return rv.Int()
	}
	if rv.CanUint() {
		return int64(rv.Uint())
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(rv.Uint())
	case reflect.Bool:
		if rv.Bool() {
			return 1
		}
		return 0
	case reflect.Float32, reflect.Float64:
		return int64(rv.Float())
	default:
		return 0
	}
}

// materializeYaklibExportCallable builds a first-class callable for a yaklib
// export used as a value (e.g. f = poc.ReplaceHTTPPacketHeader; f(...)). The
// closure's fn is the runtime's YaklibExportCallableMarker and its freeValues
// carry (pkg, method), so invoking it dispatches through the yaklib table
// instead of calling an undefined symbol.
func (c *Compiler) materializeYaklibExportCallable(val ssa.Value, pkg, method string) llvm.Value {
	i64 := c.LLVMCtx.Int64Type()
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	i64Ptr := llvm.PointerType(i64, 0)

	pkgPtr := c.Builder.CreateGlobalStringPtr(pkg, fmt.Sprintf("yaklib_pkg_%d", val.GetId()))
	methodPtr := c.Builder.CreateGlobalStringPtr(method, fmt.Sprintf("yaklib_method_%d", val.GetId()))

	var closure llvm.Value
	c.withSSADefInsertPoint(val.GetId(), func() {
		mallocFn, mallocType := c.getOrInsertMalloc()
		raw := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{llvm.ConstInt(i64, 16, false)}, "yaklib_export_free_mem")
		freeI64Ptr := c.Builder.CreateIntToPtr(raw, i64Ptr, "yaklib_export_free_i64p")
		slot0 := c.Builder.CreateGEP(i64, freeI64Ptr, []llvm.Value{llvm.ConstInt(i64, 0, false)}, "")
		c.Builder.CreateStore(llvm.ConstPtrToInt(pkgPtr, i64), slot0)
		slot1 := c.Builder.CreateGEP(i64, freeI64Ptr, []llvm.Value{llvm.ConstInt(i64, 1, false)}, "")
		c.Builder.CreateStore(llvm.ConstPtrToInt(methodPtr, i64), slot1)

		makeFn, makeType := c.getOrInsertRuntimeMakeCallable()
		closure = c.Builder.CreateCall(makeType, makeFn, []llvm.Value{
			llvm.ConstInt(i64, abi.YaklibExportCallableMarker, false),
			llvm.ConstInt(i64, 0, false),
			llvm.ConstInt(i64, 2, false),
			freeI64Ptr,
		}, "yaklib_export_callable")
	})
	_ = i8Ptr
	return closure
}
