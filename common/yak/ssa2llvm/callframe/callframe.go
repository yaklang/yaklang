package callframe

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

type FreeValueBinding struct {
	Variable *ssa.Variable
	Name     string
	ValueID  int64
}

type FrameInputKind uint8

const (
	FrameInputParam FrameInputKind = iota
	FrameInputParamMember
	FrameInputFreeValue
)

type FrameInput struct {
	Kind         FrameInputKind
	Index        int
	Variable     *ssa.Variable
	Value        ssa.Value
	FunctionLike bool
}

func OrderedFreeValueBindings(fn *ssa.Function) []FreeValueBinding {
	if fn == nil || len(fn.FreeValues) == 0 {
		return nil
	}

	bindings := make([]FreeValueBinding, 0, len(fn.FreeValues))
	for variable, valueID := range fn.FreeValues {
		if variable == nil || valueID <= 0 {
			continue
		}
		bindings = append(bindings, FreeValueBinding{
			Variable: variable,
			Name:     variable.GetName(),
			ValueID:  valueID,
		})
	}

	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Name == bindings[j].Name {
			return bindings[i].ValueID < bindings[j].ValueID
		}
		return bindings[i].Name < bindings[j].Name
	})
	return bindings
}

func OrderedCallFrameInputs(fn *ssa.Function) []FrameInput {
	if fn == nil {
		return nil
	}

	inputs := make([]FrameInput, 0, len(fn.Params)+len(fn.ParameterMembers)+len(fn.FreeValues))
	for index, valueID := range fn.Params {
		value, _ := fn.GetValueById(valueID)
		inputs = append(inputs, FrameInput{
			Kind:         FrameInputParam,
			Index:        index,
			Value:        value,
			FunctionLike: IsFunctionLikeValue(value),
		})
	}
	for index, valueID := range fn.ParameterMembers {
		value, _ := fn.GetValueById(valueID)
		inputs = append(inputs, FrameInput{
			Kind:         FrameInputParamMember,
			Index:        index,
			Value:        value,
			FunctionLike: IsFunctionLikeValue(value),
		})
	}
	for _, binding := range OrderedFreeValueBindings(fn) {
		value, _ := fn.GetValueById(binding.ValueID)
		inputs = append(inputs, FrameInput{
			Kind:         FrameInputFreeValue,
			Variable:     binding.Variable,
			Value:        value,
			FunctionLike: IsFunctionLikeValue(value),
		})
	}
	return inputs
}

func IsFunctionLikeValue(value ssa.Value) bool {
	if value == nil {
		return false
	}
	if inst, ok := value.(ssa.Instruction); ok && inst.IsLazy() {
		if self, ok := inst.Self().(ssa.Value); ok && self != nil {
			value = self
		}
	}
	if fn, ok := ssa.ToFunction(value); ok && fn != nil {
		return true
	}
	if param, ok := ssa.ToParameter(value); ok && param != nil && param.GetDefault() != nil {
		if fn, ok := ssa.ToFunction(param.GetDefault()); ok && fn != nil {
			return true
		}
	}
	return false
}

func ResolveDirectCallee(program *ssa.Program, fn *ssa.Function, call *ssa.Call) (*ssa.Function, bool) {
	if fn == nil || call == nil {
		return nil, false
	}

	calleeVal, ok := fn.GetValueById(call.Method)
	if !ok || calleeVal == nil {
		return nil, false
	}
	if ssaFn, ok := ssa.ToFunction(calleeVal); ok && ssaFn != nil && !ssaFn.IsExtern() {
		return ssaFn, true
	}
	if ft, ok := calleeVal.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
		return ft.This, true
	}
	if param, ok := ssa.ToParameter(calleeVal); ok && param != nil {
		if defVal := param.GetDefault(); defVal != nil {
			if ssaFn, ok := ssa.ToFunction(defVal); ok && ssaFn != nil && !ssaFn.IsExtern() {
				return ssaFn, true
			}
			if ft, ok := defVal.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
				return ft.This, true
			}
		}
	}
	if !calleeVal.IsMember() {
		return resolveFunctionByName(calleeVal.GetName(), program)
	}
	ft, ok := calleeVal.GetType().(*ssa.FunctionType)
	if !ok || ft == nil || ft.This == nil || ft.This.IsExtern() {
		return resolveFunctionByName(calleeVal.GetName(), program)
	}
	return ft.This, true
}

func BuildCallFrameArgIDs(program *ssa.Program, call *ssa.Call, calleeFn *ssa.Function) []int64 {
	if call == nil {
		return nil
	}

	frameInputs := OrderedCallFrameInputs(calleeFn)
	if len(frameInputs) == 0 {
		return nil
	}

	zero := ensureProgramZeroConst(program)
	argIDs := make([]int64, 0, len(frameInputs))
	for _, input := range frameInputs {
		switch input.Kind {
		case FrameInputParam:
			if input.Index < len(call.Args) && call.Args[input.Index] > 0 {
				argIDs = append(argIDs, call.Args[input.Index])
			} else {
				argIDs = append(argIDs, zero)
			}
		case FrameInputParamMember:
			if input.Index < len(call.ArgMember) && call.ArgMember[input.Index] > 0 {
				actualID := call.ArgMember[input.Index]
				if shouldDeferParameterMemberArg(call, calleeFn, input, actualID) {
					argIDs = append(argIDs, zero)
				} else {
					argIDs = append(argIDs, actualID)
				}
			} else {
				argIDs = append(argIDs, zero)
			}
		case FrameInputFreeValue:
			if input.Variable != nil {
				if actualID, ok := call.Binding[input.Variable.GetName()]; ok && actualID > 0 {
					argIDs = append(argIDs, actualID)
					continue
				}
			}
			argIDs = append(argIDs, zero)
		default:
			argIDs = append(argIDs, zero)
		}
	}
	return argIDs
}

func shouldDeferParameterMemberArg(call *ssa.Call, calleeFn *ssa.Function, input FrameInput, actualID int64) bool {
	if call == nil || calleeFn == nil || input.Kind != FrameInputParamMember || input.Value == nil || actualID <= 0 {
		return false
	}

	formalMember, ok := ssa.ToParameterMember(input.Value)
	if !ok || formalMember == nil || !formalParameterMemberUsedOnlyAsCallTarget(formalMember) {
		return false
	}

	callerFn := call.GetFunc()
	if callerFn == nil {
		return false
	}
	actualVal, ok := callerFn.GetValueById(actualID)
	if !ok || actualVal == nil || !actualVal.IsMember() {
		return false
	}

	return !ssaValueResolvesToDirectCallable(actualVal)
}

func formalParameterMemberUsedOnlyAsCallTarget(member *ssa.ParameterMember) bool {
	if member == nil || !member.HasUsers() {
		return false
	}

	sawCallTarget := false
	for _, user := range member.GetUsers() {
		call, ok := ssa.ToCall(user)
		if !ok || call == nil || call.Method != member.GetId() {
			return false
		}
		sawCallTarget = true
	}
	return sawCallTarget
}

func ssaValueResolvesToDirectCallable(val ssa.Value) bool {
	if val == nil {
		return false
	}
	if ssaFn, ok := ssa.ToFunction(val); ok && ssaFn != nil && !ssaFn.IsExtern() {
		return true
	}
	if ft, ok := val.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
		return true
	}
	if param, ok := ssa.ToParameter(val); ok && param != nil && param.GetDefault() != nil {
		if ssaFn, ok := ssa.ToFunction(param.GetDefault()); ok && ssaFn != nil && !ssaFn.IsExtern() {
			return true
		}
		if ft, ok := param.GetDefault().GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
			return true
		}
	}
	return false
}

func resolveFunctionByName(name string, program *ssa.Program) (*ssa.Function, bool) {
	if program == nil {
		return nil, false
	}
	name = normalizeValueName(name)
	if name == "" {
		return nil, false
	}
	var target *ssa.Function
	program.EachFunction(func(fn *ssa.Function) {
		if target != nil || fn == nil || fn.IsExtern() || fn.GetProgram() != program {
			return
		}
		if normalizeValueName(fn.GetName()) == name {
			target = fn
		}
	})
	return target, target != nil
}

func ensureProgramZeroConst(program *ssa.Program) int64 {
	if program == nil {
		return 0
	}
	zero := ssa.NewConst(int64(0))
	zero.SetProgram(program)
	program.SetVirtualRegister(zero)
	program.AddConstInstruction(zero)
	return zero.GetId()
}

func normalizeValueName(name string) string {
	name = strings.Trim(strings.TrimSpace(name), "\"")
	if name == "" || strings.HasPrefix(name, "#") {
		return ""
	}
	return name
}
