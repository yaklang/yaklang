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
			actualID := resolveCallArgMemberID(call, calleeFn, input)
			if actualID > 0 {
				if shouldDeferParameterMemberArg(call, calleeFn, input, actualID) {
					argIDs = append(argIDs, zero)
				} else {
					argIDs = append(argIDs, actualID)
				}
			} else {
				argIDs = append(argIDs, zero)
			}
		case FrameInputFreeValue:
			resolved := false
			name := ""
			if input.Variable != nil {
				name = input.Variable.GetName()
			}
			callerFn := call.GetFunc()
			if name != "" && callerFn != nil {
				if !resolved {
					for variable, valueID := range callerFn.FreeValues {
						if variable != nil && variable.GetName() == name && valueID > 0 {
							argIDs = append(argIDs, valueID)
							resolved = true
							break
						}
					}
				}
				if !resolved {
					if actualID, ok := call.Binding[name]; ok && actualID > 0 {
						if valueBelongsToFunctionOrParent(callerFn, actualID) {
							argIDs = append(argIDs, actualID)
							resolved = true
						}
					}
				}
			}
			if !resolved {
				argIDs = append(argIDs, zero)
			}
		default:
			argIDs = append(argIDs, zero)
		}
	}
	return argIDs
}

func valueBelongsToFunctionOrParent(fn *ssa.Function, valueID int64) bool {
	for current := fn; current != nil; current = current.GetParent() {
		if value, ok := current.GetValueById(valueID); ok && value != nil {
			return true
		}
	}
	return false
}

func resolveCallArgMemberID(call *ssa.Call, calleeFn *ssa.Function, input FrameInput) int64 {
	if call == nil || input.Kind != FrameInputParamMember {
		return 0
	}

	if formal, ok := ssa.ToParameterMember(input.Value); ok && formal != nil {
		if actualID := findMatchingCallArgMember(call, calleeFn, formal); actualID > 0 {
			return actualID
		}
	}

	if input.Index < len(call.ArgMember) && call.ArgMember[input.Index] > 0 {
		return call.ArgMember[input.Index]
	}
	return 0
}

func findMatchingCallArgMember(call *ssa.Call, calleeFn *ssa.Function, formal *ssa.ParameterMember) int64 {
	if call == nil || calleeFn == nil || formal == nil {
		return 0
	}
	callerFn := call.GetFunc()
	if callerFn == nil {
		return 0
	}

	formalRoot, formalKey := formalMemberRootNameAndKey(calleeFn, formal)
	if formalRoot == "" || formalKey == "" {
		return 0
	}

	boundRootID := boundRootValueID(call, callerFn, formalRoot)
	for _, actualID := range call.ArgMember {
		if actualID <= 0 {
			continue
		}
		actual, ok := callerFn.GetValueById(actualID)
		if !ok || actual == nil || !actual.IsMember() || actual.GetObject() == nil || actual.GetKey() == nil {
			continue
		}
		if boundRootID > 0 && actual.GetObject().GetId() != boundRootID {
			continue
		}
		if memberKeyString(actual.GetKey()) == formalKey {
			return actualID
		}
	}
	return 0
}

func boundRootValueID(call *ssa.Call, callerFn *ssa.Function, name string) int64 {
	if call == nil || callerFn == nil || name == "" {
		return 0
	}
	if actualID, ok := call.Binding[name]; ok && actualID > 0 {
		if _, exists := callerFn.GetValueById(actualID); exists {
			return actualID
		}
	}
	for variable, valueID := range callerFn.FreeValues {
		if variable != nil && variable.GetName() == name && valueID > 0 {
			return valueID
		}
	}
	return 0
}

func formalMemberRootNameAndKey(fn *ssa.Function, formal *ssa.ParameterMember) (string, string) {
	if fn == nil || formal == nil || formal.GetKey() == nil {
		return "", ""
	}
	key := memberKeyString(formal.GetKey())
	if key == "" {
		return "", ""
	}
	obj := formal.GetObject()
	for obj != nil {
		if param, ok := ssa.ToParameter(obj); ok && param != nil {
			return param.GetName(), key
		}
		if pm, ok := ssa.ToParameterMember(obj); ok && pm != nil {
			obj = pm.GetObject()
			continue
		}
		return obj.GetName(), key
	}
	return "", key
}

func memberKeyString(key ssa.Value) string {
	if key == nil {
		return ""
	}
	if cinst, ok := ssa.ToConstInst(key); ok && cinst != nil {
		return strings.Trim(cinst.String(), "\"")
	}
	return strings.Trim(key.GetName(), "\"")
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
	_ = program
	return 0
}

func normalizeValueName(name string) string {
	name = strings.Trim(strings.TrimSpace(name), "\"")
	if name == "" || strings.HasPrefix(name, "#") {
		return ""
	}
	return name
}
