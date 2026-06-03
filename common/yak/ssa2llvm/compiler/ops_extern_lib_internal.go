package compiler

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
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

	var memberValID int64
	if keyStr != "" {
		if id, ok := extern.MemberMap[keyStr]; ok {
			memberValID = id
		}
	}
	if memberValID == 0 && key != nil {
		if member, ok := extern.GetMember(key); ok && member != nil {
			memberValID = member.GetId()
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
