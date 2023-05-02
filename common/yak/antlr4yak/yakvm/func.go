package yakvm

import (
	"fmt"
	"reflect"
	"yaklang/common/log"

	uuid "github.com/satori/go.uuid"
)

type Function struct {
	id                        int
	name                      string
	codes                     []*Code
	scope                     *Scope
	anonymousFunctionBindName string
	symbolTable               *SymbolTable
	uuid                      string
	paramSymbols              []int
	isVariableParameter       bool
}

func (f *Function) GetActualName() string {
	if f.name == "anonymous" && f.anonymousFunctionBindName != "" {
		return f.anonymousFunctionBindName
	}
	return f.name
}
func (f *Function) GetUUID() string {
	return f.uuid
}
func (f *Function) GetBindName() string {
	return f.anonymousFunctionBindName
}
func (f *Function) GetCodes() []*Code {
	return f.codes
}
func (f *Function) SetParamSymbols(i []int) {
	f.paramSymbols = i
}
func (f *Function) SetName(name string) {
	f.name = name
}
func (f *Function) SetSymbol(id int) {
	f.id = id
}
func (f *Function) GetSymbolId() int {
	return f.id
}
func (f *Function) GetName() string {
	return f.name
}
func (f *Function) IsVariableParameter() bool {
	return f.isVariableParameter
}
func (f *Function) SetIsVariableParameter(v bool) {
	f.isVariableParameter = v
}

func NewFunction(codes []*Code, tbl *SymbolTable) *Function {
	return &Function{
		name:        "anonymous",
		codes:       codes,
		symbolTable: tbl,
		uuid:        uuid.NewV4().String(),
	}
}

func (f *Function) String() string {
	return fmt.Sprintf("function params[%v] codes[%v]", len(f.paramSymbols), len(f.codes))
}

func YakVMValuesToFunctionMap(f *Function, vs []*Value, argumentCheck bool) map[int]*Value {
	var variableParamsId int
	var stableArgumentsNumber int

	funcName := f.GetActualName()

	if f.IsVariableParameter() {
		variableParamsId = f.paramSymbols[len(f.paramSymbols)-1]
		stableArgumentsNumber = len(f.paramSymbols) - 1
	} else {
		stableArgumentsNumber = len(f.paramSymbols)
		if argumentCheck {
			if len(f.paramSymbols) != len(vs) {

				panic(fmt.Sprintf("function %v params number not match, expect %v, got %v", funcName, len(f.paramSymbols), len(vs)))
			}
		}
	}
	//newVm := vm.CreateSubVirtualMachine(f.codes, f.symbolTable)
	params := make(map[int]*Value)
	if argumentCheck {
		if stableArgumentsNumber > len(vs) {
			panic(fmt.Sprintf("runtime error: function %s need at least %d params, got %d params", funcName, stableArgumentsNumber, len(vs)))
		}
	}
	otherVs := make([]*Value, 0)
	for _, v := range vs {
		if v.SymbolId != 0 {
			params[v.SymbolId] = v
		} else {
			otherVs = append(otherVs, v)
		}
	}
	vs = otherVs
	var i = 0
	var t = 0
	for ; t < stableArgumentsNumber; t++ {
		if t >= len(f.paramSymbols) {
			break
		}
		symbolId := f.paramSymbols[t]
		if _, ok := params[symbolId]; ok {
			continue
		}
		var valueIns *Value
		if i < len(vs) {
			valueIns = vs[i]
		} else {
			valueIns = undefined
		}
		params[symbolId] = valueIns
		i++
	}
	if f.IsVariableParameter() {
		variableParams := make([]interface{}, len(vs)-(stableArgumentsNumber))
		for j := 0; i < len(vs); i++ {
			variableParams[j] = vs[i].Value
			j++
		}
		variableParamsValue := NewValue("[]any", variableParams, "")
		params[variableParamsId] = variableParamsValue
	}
	return params
}

func LuaVMValuesToFunctionMap(f *Function, vs []*Value) map[int]*Value {
	var variableParamsId int
	var stableArgumentsNumber int

	funcName := f.GetActualName()

	// 先展开传入参数包含可变参数的情况
	newVS := make([]*Value, 0)
	for _, val := range vs {
		if val.TypeVerbose == "variadic-params" {
			mapLen := val.Len()
			valRF := reflect.ValueOf(val.Value)
			for index := 0; index < mapLen; index++ {
				tmpVal := valRF.MapIndex(reflect.ValueOf(index + 1)).Interface()
				newVS = append(newVS, NewAutoValue(tmpVal))
			}
		} else {
			newVS = append(newVS, val)
		}
	}
	vs = newVS

	if f.IsVariableParameter() {
		variableParamsId = f.paramSymbols[len(f.paramSymbols)-1]
		stableArgumentsNumber = len(f.paramSymbols) - 1
	} else {
		stableArgumentsNumber = len(f.paramSymbols)
		passedInArgumentNumber := len(vs)
		if stableArgumentsNumber != passedInArgumentNumber {
			if stableArgumentsNumber > passedInArgumentNumber {
				vs = append(vs, undefined)
			} else {
				vs = vs[:stableArgumentsNumber]
			}
		}
	}
	//newVm := vm.CreateSubVirtualMachine(f.codes, f.symbolTable)
	params := make(map[int]*Value)
	if stableArgumentsNumber > len(vs) {
		log.Warn(fmt.Sprintf("runtime error: function %s need at least %d params, got %d params", funcName, stableArgumentsNumber, len(vs)))
		for stableArgumentsNumber > len(vs) {
			vs = append(vs, undefined)
		}
	}
	var i = 0
	for ; i < stableArgumentsNumber; i++ {
		symbolId := f.paramSymbols[i]
		valueIns := vs[i]
		params[symbolId] = valueIns
		//newVm.CurrentScope().NewValueByID(symbolId, valueIns)
	}

	if f.IsVariableParameter() {
		variableParams := make(map[int]interface{}, len(vs)-(stableArgumentsNumber))
		for j := 0; i < len(vs); i++ {
			variableParams[j+1] = vs[i].Value
			j++
		}
		if len(variableParams) == 0 {
			params[variableParamsId] = undefined
		} else {
			variableParamsValue := NewValue("variadic-params", variableParams, "[int]any")
			params[variableParamsId] = variableParamsValue
		}
	}
	return params
}

func (vm *Frame) CallYakFunction(asyncCall bool, f *Function, vs []*Value) interface{} {
	params := YakVMValuesToFunctionMap(f, vs, vm.vm.config.GetFunctionNumberCheck())

	if asyncCall {
		vm.vm.ExecAsyncYakFunction(vm.ctx, f, params)
		return nil
	}
	v, _ := vm.vm.ExecYakFunction(vm.ctx, f, params)

	return v
}

func (vm *Frame) CallLuaFunction(asyncCall bool, f *Function, vs []*Value) interface{} {
	params := LuaVMValuesToFunctionMap(f, vs)

	// TODO: 目前lua暂不支持coroutine
	//if asyncCall {
	//	vm.vm.ExecAsyncYakFunction(f, params)
	//	return nil
	//}
	v, _ := vm.vm.ExecYakFunction(vm.ctx, f, params)

	return v
}
