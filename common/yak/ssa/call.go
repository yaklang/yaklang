package ssa

import (
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func NewCall(target Value, args []Value, binding map[string]Value, block *BasicBlock) *Call {
	if binding == nil {
		binding = make(map[string]Value)
	}

	c := &Call{
		anValue:     NewValue(),
		Method:      target,
		Args:        args,
		Binding:     binding,
		Async:       false,
		Unpack:      false,
		IsDropError: false,
		IsEllipsis:  false,
	}
	return c
}

func (f *FunctionBuilder) NewCall(target Value, args []Value) *Call {
	call := NewCall(target, args, nil, f.CurrentBlock)
	return call
}

func (f *FunctionBuilder) EmitCall(c *Call) *Call {
	if f.CurrentBlock.finish {
		return nil
	}

	f.emit(c)
	c.handlerGeneric()
	c.handlerObjectMethod()
	c.handlerReturnType()
	c.handleCalleeFunction()

	return c
}

func (c *Call) handlerGeneric() {
	newMethod := c.Method
	if !newMethod.IsExtern() || newMethod.GetOpcode() != SSAOpcodeFunction {
		return
	}
	f := newMethod.(*Function)
	if !f.isGeneric {
		return
	}

	fType := f.Type
	genericTypes := make(map[Type]struct{}, 0) // for calculate cache name
	for _, typ := range fType.Parameter {
		types := GetGenericTypeFromType(typ)
		for _, typ := range types {
			if _, ok := genericTypes[typ]; !ok {
				genericTypes[typ] = struct{}{}
			}
		}
	}

	if len(genericTypes) == 0 {
		return
	}

	// binding generic type
	isVariadic := fType.IsVariadic
	genericTypeMap := make(map[string]Type, len(genericTypes))
	paramsType := make([]Type, fType.ParameterLen)
	returnType := fType.ReturnType

	hasError := false
	for i, arg := range c.Args {
		index := i
		if isVariadic && i > fType.ParameterLen-1 {
			index = fType.ParameterLen - 1
		} else {
			// variadic should not set new paramsType
			paramsType[i] = arg.GetType()
		}

		errMsg := BindingGenericTypeWithRealType(arg.GetType(), fType.Parameter[index], genericTypeMap)
		if errMsg != "" && !hasError {
			hasError = true
			c.NewError(Error, SSATAG, errMsg)
		}
	}

	// calculate cache name
	var nameBuilder strings.Builder
	nameBuilder.WriteString(newMethod.GetName())
	keys := lo.Keys(genericTypeMap)
	sort.Strings(keys)
	for _, k := range keys {
		nameBuilder.WriteRune('-')
		nameBuilder.WriteString(genericTypeMap[k].String())
	}
	name := nameBuilder.String()

	prog := c.GetProgram()
	if prog == nil {
		log.Errorf("[ssa.Call.handlerGeneric] Can't found ssa program")
		return
	}

	if value, ok := prog.GetCacheExternInstance(name); !ok {
		// apply generic type
		returnType = ApplyGenericType(returnType, genericTypeMap)
		// create new function type and set cache
		newFuncTyp := NewFunctionType(newMethod.GetName(), paramsType, returnType, fType.IsVariadic)
		newMethod = NewFunctionWithType(newMethod.GetName(), newFuncTyp)
		prog.SetCacheExternInstance(name, newMethod)
	} else {
		newMethod = value
	}
	c.Method = newMethod
}

func (c *Call) handlerObjectMethod() {
	args := c.Args
	target := c.Method
	// handler "this" in parameter
	AddThis := func(this Value) {
		if len(args) == 0 {
			args = append(args, this)
		} else {
			args = utils.InsertSliceItem(args, this, 0)
		}
	}
	switch t := target.GetType().(type) {
	case *FunctionType:
		if t.IsMethod {
			if obj := target.GetObject(); obj != nil {
				AddThis(obj)
			} else {
				//  is method but not object
				log.Errorf("method call, but object is nil")
			}
		}
	default:
		_ = t
	}
	c.Args = args
}

// handler Return type, and handle drop error
func (c *Call) handlerReturnType() {
	// get function type
	funcTyp, ok := ToFunctionType(c.Method.GetType())
	if !ok {
		return
	}
	// inference call instruction type
	if c.IsDropError {
		if retType, ok := funcTyp.ReturnType.(*ObjectType); ok {
			if retType.Combination && retType.FieldTypes[len(retType.FieldTypes)-1].GetTypeKind() == ErrorTypeKind {
				if len(retType.FieldTypes) == 1 {
					c.SetType(BasicTypes[NullTypeKind])
				} else if len(retType.FieldTypes) == 2 {
					// if len(t.FieldTypes) == 2 {
					c.SetType(retType.FieldTypes[0])
				} else {
					ret := NewStructType()
					ret.FieldTypes = retType.FieldTypes[:len(retType.FieldTypes)-1]
					ret.Keys = retType.Keys[:len(retType.Keys)-1]
					ret.KeyTyp = retType.KeyTyp
					ret.Combination = true
					ret.Len = len(ret.FieldTypes)
					ret.Kind = TupleTypeKind
					c.SetType(ret)
				}
				return
			}
		} else if t, ok := funcTyp.ReturnType.(*BasicType); ok && t.Kind == ErrorTypeKind {
			// pass
			c.SetType(BasicTypes[NullTypeKind])
			for _, variable := range c.GetAllVariables() {
				variable.NewError(Error, SSATAG, ValueIsNull())
			}
			return
		}
		c.NewError(Warn, SSATAG, FunctionContReturnError())
	} else {
		c.SetType(funcTyp.ReturnType)
	}

	// handler free value
	c.HandleFreeValue(funcTyp.FreeValue)
}

// handler if method, set object for first argument
func (c *Call) handleCalleeFunction() {
	// get function type
	funcTyp, ok := ToFunctionType(c.Method.GetType())
	if !ok {
		return
	}

	{
		builder := c.GetFunc().builder
		recoverBuilder := builder.SetCurrent(c)
		currentScope := c.GetBlock().ScopeTable

		for true {
			if len(c.ArgMember) == len(funcTyp.ParameterMember) {
				break
			}
			for _, p := range funcTyp.ParameterMember {

				objectName := p.ObjectName
				key := p.MemberCallKey
				object, ok := p.Get(c)
				if !ok {
					continue
				}

				if _, typ := checkCanMemberCall(object, key); typ == nil {
					builder.NewErrorWithPos(Error, SSATAG,
						p.GetRange(),
						FreeValueNotMember(
							objectName,
							p.MemberCallKey.String(),
							c.GetRange(),
						),
					)
					c.NewError(Error, SSATAG,
						FreeValueNotMemberInCall(
							objectName,
							p.MemberCallKey.String(),
						),
					)
					continue
				}
				c.ArgMember = append(c.ArgMember,
					builder.ReadMemberCallVariable(object, key),
				)
			}
			break
		}

		// handle side effect
		for _, se := range funcTyp.SideEffects {
			var variable *Variable
			if se.MemberCallKind == NoMemberCall {
				// side-effect only create in scope that lower or same than modify's scope
				if !se.forceCreate && !currentScope.IsSameOrSubScope(se.Variable.GetScope()) {
					continue
				}
				// is normal side-effect
				variable = builder.CreateVariable(se.Name)
			} else {
				// is object
				obj, ok := se.Get(c)
				if !ok {
					continue
				}
				variable = builder.CreateMemberCallVariable(obj, se.MemberCallKey)
			}

			// TODO: handle side effect in loop scope,
			// will replace value in scope and create new phi
			sideEffect := builder.EmitSideEffect(se.Name, c, se.Modify)
			if sideEffect != nil {
				builder.AssignVariable(variable, sideEffect)
				sideEffect.SetVerboseName(se.VerboseName)
			}
		}
		recoverBuilder()
	}

	// only handler in method call
	if !funcTyp.IsMethod {
		return
	}

	is := c.Method.IsMember()
	if !is {
		// this function is method Function, but no member call get this.
		// error
		return
	}
}

func (c *Call) HandleFreeValue(fvs []*Parameter) {
	builder := c.GetFunc().builder
	recoverBuilder := builder.SetCurrent(c)
	defer recoverBuilder()

	for _, fv := range fvs {
		// if freeValue has default value, skip
		if fv.GetDefault() != nil {
			c.Binding[fv.GetName()] = fv.GetDefault()
			continue
		}

		v := builder.PeekValue(fv.GetName())

		if v != nil {
			c.Binding[fv.GetName()] = v
		} else {
			// mark error in freeValue.Variable
			// get freeValue
			if variable := fv.GetVariable(fv.GetName()); variable != nil {
				variable.NewError(Error, SSATAG, BindingNotFound(fv.GetName(), c.GetRange()))
			}
			// skip instance function, or `go` with instance function,
			// this function no variable, and code-range of call-site same as function.
			// we don't mark error in call-site.
			if fun, ok := ToFunction(c.Method); ok {
				if len(fun.GetAllVariables()) == 0 {
					continue
				}
			}
			// other code will mark error in function call-site
			c.NewError(Error, SSATAG, BindingNotFoundInCall(fv.GetName()))
		}
	}
}
