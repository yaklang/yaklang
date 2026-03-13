package compiler

import (
	"fmt"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) coerceToI1(val llvm.Value, name string) llvm.Value {
	if val.Type().IntTypeWidth() == 1 {
		return val
	}
	zero := llvm.ConstInt(val.Type(), 0, false)
	return c.Builder.CreateICmp(llvm.IntNE, val, zero, name)
}

func (c *Compiler) resolveCalleeName(fn *ssa.Function, methodID int64) string {
	if fn != nil {
		if calleeVal, ok := fn.GetValueById(methodID); ok && calleeVal != nil {
			if name := c.resolveValueName(fn, calleeVal); name != "" {
				return name
			}
		}
	}
	return fmt.Sprintf("func_%d", methodID)
}

func (c *Compiler) resolveValueName(fn *ssa.Function, val ssa.Value) string {
	if val == nil {
		return ""
	}
	if ssaFn, ok := ssa.ToFunction(val); ok {
		if name := normalizeResolvedValueName(ssaFn.GetName()); name != "" {
			return name
		}
	}
	if mc, ok := val.(ssa.MemberCall); ok && mc.IsMember() {
		objName := c.resolveMemberObjectName(fn, ssa.GetLatestObject(val))
		keyName := c.resolveMemberKeyString(ssa.GetLatestKey(val))
		switch {
		case objName != "" && keyName != "":
			return objName + "." + keyName
		case keyName != "":
			return keyName
		}
	}
	return normalizeResolvedValueName(val.GetName())
}

func (c *Compiler) resolveMemberObjectName(fn *ssa.Function, obj ssa.Value) string {
	if obj == nil {
		return ""
	}
	if name := c.resolveValueName(fn, obj); name != "" {
		return name
	}
	if fn != nil {
		if resolved, ok := fn.GetValueById(obj.GetId()); ok && resolved != nil && resolved != obj {
			return c.resolveValueName(fn, resolved)
		}
	}
	return ""
}

func normalizeResolvedValueName(name string) string {
	name = strings.Trim(strings.TrimSpace(name), "\"")
	if name == "" || strings.HasPrefix(name, "#") {
		return ""
	}
	return name
}
