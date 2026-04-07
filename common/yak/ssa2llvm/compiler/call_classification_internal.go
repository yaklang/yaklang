package compiler

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
)

const (
	callLowerTagInternal = "call:internal"
	callLowerTagDispatch = "call:dispatch"
	callLowerTagExtern   = "call:extern"
)

func PrepareCallLoweringTags(program *ssa.Program, externBindings map[string]ExternBinding, tags map[int64]string) error {
	if program == nil || tags == nil {
		return nil
	}

	for id, tag := range tags {
		switch tag {
		case callLowerTagInternal, callLowerTagDispatch, callLowerTagExtern:
			delete(tags, id)
		}
	}

	mergedBindings := mergeExternBindings(defaultExternBindings, externBindings)
	var firstErr error
	program.EachFunction(func(fn *ssa.Function) {
		if firstErr != nil || fn == nil || fn.IsExtern() {
			return
		}
		for _, blockID := range fn.Blocks {
			block, ok := getFunctionBasicBlock(fn, blockID)
			if !ok || block == nil {
				continue
			}
			for _, instID := range append([]int64(nil), block.Insts...) {
				inst, ok := fn.GetInstructionById(instID)
				if !ok || inst == nil {
					continue
				}
				if inst.IsLazy() {
					inst = inst.Self()
				}
				call, ok := ssa.ToCall(inst)
				if !ok || call == nil {
					continue
				}
				tag, err := classifyCallLoweringTag(program, fn, call, mergedBindings)
				if err != nil {
					firstErr = err
					return
				}
				if tag != "" {
					tags[call.GetId()] = tag
				}
			}
		}
	})
	return firstErr
}

func classifyCallLoweringTag(program *ssa.Program, fn *ssa.Function, call *ssa.Call, externBindings map[string]ExternBinding) (string, error) {
	if program == nil || fn == nil || call == nil {
		return "", nil
	}
	if calleeFn, ok := callframe.ResolveDirectCallee(program, fn, call); ok && calleeFn != nil && calleeFn.GetProgram() == program && !calleeFn.IsExtern() {
		return callLowerTagInternal, nil
	}

	calleeVal, _ := fn.GetValueById(call.Method)
	if mc, ok := calleeVal.(ssa.MemberCall); ok && mc.IsMember() {
		return "", nil
	}

	calleeName := resolveCallSiteName(fn, call.Method)
	if binding, ok := externBindings[calleeName]; ok && binding.DispatchID != 0 {
		return callLowerTagDispatch, nil
	}
	if binding, ok := externBindings[calleeName]; ok && binding.Symbol != "" {
		if err := validateExternBindingCallABI(calleeName, binding); err != nil {
			return "", err
		}
		return callLowerTagExtern, nil
	}
	return "", nil
}

func getFunctionBasicBlock(fn *ssa.Function, blockID int64) (*ssa.BasicBlock, bool) {
	if fn == nil || blockID <= 0 {
		return nil, false
	}
	val, ok := fn.GetValueById(blockID)
	if !ok || val == nil {
		return nil, false
	}
	block, ok := ssa.ToBasicBlock(val)
	return block, ok && block != nil
}

func resolveCallSiteName(fn *ssa.Function, methodID int64) string {
	if fn != nil {
		if calleeVal, ok := fn.GetValueById(methodID); ok && calleeVal != nil {
			if name := resolveSSAValueName(fn, calleeVal); name != "" {
				return name
			}
		}
	}
	return fmt.Sprintf("func_%d", methodID)
}

func resolveSSAValueName(fn *ssa.Function, val ssa.Value) string {
	if val == nil {
		return ""
	}
	if ssaFn, ok := ssa.ToFunction(val); ok && ssaFn != nil {
		if name := normalizeSSAResolvedValueName(ssaFn.GetName()); name != "" {
			return name
		}
	}
	if mc, ok := val.(ssa.MemberCall); ok && mc.IsMember() {
		objName := resolveSSAMemberObjectName(fn, ssa.GetLatestObject(val))
		keyName := resolveSSAMemberKeyString(ssa.GetLatestKey(val))
		switch {
		case objName != "" && keyName != "":
			return objName + "." + keyName
		case keyName != "":
			return keyName
		}
	}
	return normalizeSSAResolvedValueName(val.GetName())
}

func resolveSSAMemberObjectName(fn *ssa.Function, obj ssa.Value) string {
	if obj == nil {
		return ""
	}
	if name := resolveSSAValueName(fn, obj); name != "" {
		return name
	}
	if fn != nil {
		if resolved, ok := fn.GetValueById(obj.GetId()); ok && resolved != nil && resolved != obj {
			return resolveSSAValueName(fn, resolved)
		}
	}
	return ""
}

func resolveSSAMemberKeyString(key ssa.Value) string {
	if key == nil {
		return ""
	}
	if cinst, ok := ssa.ToConstInst(key); ok {
		return strings.Trim(cinst.String(), "\"")
	}
	return strings.Trim(key.GetName(), "\"")
}

func normalizeSSAResolvedValueName(name string) string {
	name = strings.Trim(strings.TrimSpace(name), "\"")
	if name == "" || strings.HasPrefix(name, "#") {
		return ""
	}
	return name
}
