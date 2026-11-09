package ssapass

import (
	"github.com/yaklang/yaklang/common/yak/reify/core/funcdefs"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Options controls RedefineTypeValue.
type Options struct {
	stringAt func(uint64) (string, bool)
}

// Option customizes RedefineTypeValue.
type Option func(*Options)

// WithStringAt supplies the reader that resolves a constant address to a
// printable string.
func WithStringAt(stringAt func(uint64) (string, bool)) Option {
	return func(options *Options) {
		options.stringAt = stringAt
	}
}

// RedefineTypeValue replaces constant address arguments of string parameters
// with string constants. Variadic arguments are replaced only when the address
// resolves to a printable string. It returns the number of replaced arguments.
func RedefineTypeValue(program *ssa.Program, options ...Option) int {
	settings := Options{}
	for _, option := range options {
		option(&settings)
	}
	if settings.stringAt == nil {
		return 0
	}
	internalTypes := internalParameterTypes(program)
	replaced := 0
	eachCall(program, func(function *ssa.Function, call *ssa.Call) {
		definition, knownDefinition := definitionForCall(call)
		name := internalCallName(call)
		types := internalTypes[name]
		for index, argID := range call.Args {
			isString := false
			if knownDefinition {
				param, known := parameterAt(definition, index)
				if !known && !definition.Variadic {
					break
				}
				isString = known && param.Type.IsString()
				if !isString && !definition.Variadic {
					continue
				}
				// Variadic C arguments are resolved only when they point at a string.
			} else if index < len(types) {
				isString = isStringType(types[index])
			}
			if !isString {
				continue
			}
			address, ok := constantAddress(call, argID)
			if !ok {
				continue
			}
			text, ok := settings.stringAt(address)
			if !ok {
				continue
			}
			replacement := stringConstant(function, call.GetBlock(), text)
			if replacement == nil {
				continue
			}
			call.Args[index] = replacement.GetId()
			replaced++
		}
	})
	return replaced
}

func internalParameterTypes(program *ssa.Program) map[string][]ssa.Type {
	result := make(map[string][]ssa.Type)
	if program == nil {
		return result
	}
	program.Funcs.ForEach(func(name string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		types := make([]ssa.Type, 0, len(function.Params))
		for _, id := range function.Params {
			value, ok := function.GetValueById(id)
			if !ok || value == nil {
				continue
			}
			index := len(types)
			if parameter, ok := ssa.ToParameter(value); ok && parameter != nil && parameter.FormalParameterIndex >= 0 {
				index = parameter.FormalParameterIndex
			}
			for len(types) <= index {
				types = append(types, nil)
			}
			types[index] = value.GetType()
		}
		if len(types) > 0 {
			result[name] = types
		}
		return true
	})
	return result
}

func isStringType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	return typ.String() == "string" || typ.String() == "char *" || typ.String() == "const char *"
}

func parameterAt(definition funcdefs.Definition, index int) (funcdefs.Param, bool) {
	if index < 0 || index >= len(definition.Params) {
		return funcdefs.Param{}, false
	}
	return definition.Params[index], true
}

func constantAddress(call *ssa.Call, id int64) (uint64, bool) {
	value, ok := call.GetValueById(id)
	if !ok || value == nil {
		return 0, false
	}
	for depth := 0; depth < 8; depth++ {
		switch current := value.(type) {
		case *ssa.TypeCast:
			next, ok := call.GetValueById(current.Value)
			if !ok || next == nil {
				return 0, false
			}
			value = next
		case *ssa.ConstInst:
			return constAddress(current)
		default:
			return 0, false
		}
	}
	return 0, false
}

func constAddress(constant *ssa.ConstInst) (uint64, bool) {
	switch value := constant.GetRawValue().(type) {
	case int64:
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	case uint64:
		return value, true
	case int:
		if value < 0 {
			return 0, false
		}
		return uint64(value), true
	case uint32:
		return uint64(value), true
	default:
		return 0, false
	}
}

// stringConstant registers a string constant without inserting it into the
// block instruction list. It is used only as a call argument.
func stringConstant(function *ssa.Function, block *ssa.BasicBlock, text string) *ssa.ConstInst {
	if function == nil {
		return nil
	}
	constant := ssa.NewConst(text)
	constant.SetFunc(function)
	if block != nil {
		constant.SetBlock(block)
	}
	function.GetProgram().SetVirtualRegister(constant)
	return constant
}
