package ssa

import (
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func NewCall(target Value, args []Value, binding map[string]Value, block *BasicBlock) *Call {
	if binding == nil {
		binding = make(map[string]Value)
	}
	c := &Call{
		anValue:         NewValue(),
		Method:          target,
		Args:            args,
		Binding:         binding,
		Async:           false,
		Unpack:          false,
		IsDropError:     false,
		IsEllipsis:      false,
		SideEffectValue: map[string]Value{},
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
	instanceTypeMap := make(map[string]Type, len(genericTypes))
	paramsType := slices.Clone(fType.Parameter)
	returnType := fType.ReturnType

	hasError := false
	for i, arg := range c.Args {
		index := i
		argTyp := arg.GetType()
		if isVariadic && i > fType.ParameterLen-1 {
			index = fType.ParameterLen - 1
		} else {
			// variadic should not set new paramsType
			paramsType[i] = arg.GetType()
		}

		// unpack T...([]T) to T
		if i >= fType.ParameterLen-1 && c.IsEllipsis {
			if argTyp.GetTypeKind() == SliceTypeKind {
				argTyp = argTyp.(*ObjectType).FieldType
			} else if argTyp.GetTypeKind() == BytesTypeKind {
				argTyp = GetByteType()
			} else {
				// todo: should error
			}
		}

		errMsg := BindingGenericTypeWithRealType(argTyp, fType.Parameter[index], instanceTypeMap)
		if errMsg != "" && !hasError {
			hasError = true
			c.NewError(Error, SSATAG, errMsg)
		}
	}
	// fallback
	for typ := range genericTypes {
		if _, ok := instanceTypeMap[typ.String()]; !ok {
			instanceTypeMap[typ.String()] = GetAnyType()
		}
	}

	// if not enough parameter, apply generic type as any type
	if len(c.Args) < fType.ParameterLen {
		for i := len(c.Args); i < fType.ParameterLen; i++ {
			paramsType[i] = ApplyGenericType(paramsType[i], instanceTypeMap)
		}
	}

	// calculate cache name
	var nameBuilder strings.Builder
	nameBuilder.WriteString(newMethod.GetName())
	keys := lo.Keys(instanceTypeMap)
	sort.Strings(keys)
	for _, k := range keys {
		nameBuilder.WriteRune('-')
		nameBuilder.WriteString(instanceTypeMap[k].String())
	}
	name := nameBuilder.String()

	prog := c.GetProgram()
	if prog == nil {
		log.Errorf("[ssa.Call.handlerGeneric] Can't found ssa program")
		return
	}

	if value, ok := prog.GetCacheExternInstance(name); !ok {
		// apply generic type
		returnType = ApplyGenericType(returnType, instanceTypeMap)
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
		this.AddUser(c)
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
		if param, ok := ToParameter(c.Method); ok {
			caller := param.GetFunc()
			callee := caller.anValue.GetFunc()
			caller.SideEffects = append(caller.SideEffects, callee.SideEffects...)

		}
		return
	}

	{
		function := c.GetFunc()
		builder := function.builder
		recoverBuilder := builder.SetCurrent(c)
		currentScope := c.GetBlock().ScopeTable
		defer func() {
			recoverBuilder()
		}()

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

				if res := checkCanMemberCallExist(object, key); !res.exist {
					builder.NewErrorWithPos(Error, SSATAG,
						p.GetRange(),
						ValueNotMember(
							object.GetOpcode(),
							objectName,
							p.MemberCallKey.String(),
							c.GetRange(),
						),
					)
					c.NewError(Error, SSATAG,
						ValueNotMemberInCall(
							objectName,
							p.MemberCallKey.String(),
						),
					)
					continue
				}
				val := builder.ReadMemberCallValue(object, key)
				val.AddUser(c)
				c.ArgMember = append(c.ArgMember, val)
			}
			break
		}

		for _, se := range funcTyp.SideEffects {
			var variable *Variable
			var bindScope, modifyScope ScopeIF
			if se.BindVariable != nil {
				// BindVariable大多数时候和Variable相同，除非遇到object
				bindScope = se.BindVariable.GetScope()
			} else if se.Variable != nil {
				bindScope = se.Variable.GetScope()
			} else {
				bindScope = currentScope
			}
			modifyScope = se.Modify.GetBlock().ScopeTable
			_ = modifyScope

			// is object
			if se.MemberCallKind == NoMemberCall {
				if ret := GetFristLocalVariableFromScopeAndParent(currentScope, se.Name); ret != nil {
					if modifyScope.IsSameOrSubScope(ret.GetScope()) {
						continue
					}
				}

				variable = builder.CreateVariableForce(se.Name)
				if se.BindVariable != nil {
					variable.SetCaptured(se.BindVariable)
				}
			} else {
				// is object
				obj, ok := se.Get(c)
				if !ok {
					continue
				}
				variable = builder.CreateMemberCallVariable(obj, se.MemberCallKey)
				if se.BindVariable != nil {
					variable.SetCaptured(se.BindVariable)
				}
			}

			if sideEffect := builder.EmitSideEffect(se.Name, c, se.Modify); sideEffect != nil {
				if builder.SupportClosure {
					if parentValue, ok := builder.getParentFunctionVariable(se.Name); ok && se.BindVariable != nil {
						// the ret variable should be FreeValue
						para := builder.BuildFreeValueByVariable(se.BindVariable)
						para.SetDefault(parentValue)
						para.SetType(parentValue.GetType())
						parentValue.AddOccultation(para)
					}
				}

				AddSideEffect := func() {
					// TODO: handle side effect in loop scope,
					// will replace value in scope and create new phi
					sideEffect = builder.SwitchFreevalueInSideEffect(se.Name, sideEffect)
					builder.AssignVariable(variable, sideEffect)
					sideEffect.SetVerboseName(se.VerboseName)
					c.SideEffectValue[se.VerboseName] = sideEffect
				}

				SetCapturedSideEffect := func() {
					err := variable.Assign(sideEffect)
					if err != nil {
						log.Warnf("BUG: variable.Assign error: %v", err)
						return
					}
					sideEffect.SetVerboseName(se.VerboseName)
					currentScope.SetCapturedSideEffect(se.VerboseName, variable, se.BindVariable)

					function.SideEffects = append(function.SideEffects, se)
				}

				CheckSideEffect := func(find *Variable) {
					Check := func(scope ScopeIF) {
						if bindScope.IsSameOrSubScope(scope) {
							AddSideEffect()
						} else {
							SetCapturedSideEffect()
						}
					}

					if freevalue, ok := ToParameter(find.Value); ok {
						if defaultValue := freevalue.defaultValue; defaultValue != nil {
							scope := defaultValue.GetBlock().ScopeTable
							Check(scope)
							return
						}
					}
					Check(find.GetScope())
				}

				var GetScope func(ScopeIF, string, *FunctionBuilder) *Variable
				GetScope = func(scope ScopeIF, name string, builder *FunctionBuilder) *Variable {
					var ret *Variable
					if vairable := GetFristLocalVariableFromScopeAndParent(scope, name); vairable != nil {
						ret = vairable
					} else if vairable := GetFristVariableFromScopeAndParent(scope, name); vairable != nil {
						ret = vairable
					}
					if ret == nil {
						return nil
					}
					if _, ok := ToParameter(ret.GetValue()); ok {
						parentBuilder := builder.parentBuilder
						if parentBuilder != nil {
							parentScope := parentBuilder.CurrentBlock.ScopeTable
							return GetScope(parentScope, name, parentBuilder)
						}
					}

					return ret
				}

				if _, ok := se.Modify.(*Parameter); ok {
					AddSideEffect()
					continue
				}

				obj := se.parameterMemberInner
				if ret := GetScope(currentScope, se.Name, builder); ret != nil {
					CheckSideEffect(ret)
					continue
				} else if ret := GetScope(currentScope, obj.ObjectName, builder); ret != nil {
					CheckSideEffect(ret)
					continue
				} else if obj.ObjectName == "this" {
					AddSideEffect()
					continue
				}

				if obj.MemberCallKind == ParameterMemberCall || obj.MemberCallKind == CallMemberCall {
					AddSideEffect()
					continue
				}

				// 处理跨闭包的side-effect
				if block := function.GetBlock(); block != nil {
					functionScope := block.ScopeTable
					if ret := GetScope(functionScope, se.Name, builder); ret != nil {
						CheckSideEffect(ret)
						continue
					} else if obj := se.parameterMemberInner; obj.ObjectName != "" { // 处理object
						if ret := GetScope(functionScope, obj.ObjectName, builder); ret != nil {
							CheckSideEffect(ret)
							continue
						} else {
							AddSideEffect()
							continue
						}
					}
				}
			}
		}
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

	bindAndHandler := func(name string, val Value) {
		val.AddUser(c)
		c.Binding[name] = val
	}

	for _, fv := range fvs {
		// if freeValue has default value, skip
		if fv.GetDefault() != nil {
			bindAndHandler(fv.GetName(), fv.GetDefault())
			continue
		}

		value := builder.PeekValue(fv.GetName())

		if value != nil {
			bindAndHandler(fv.GetName(), value)
			//c.Binding[fv.GetName()] = v
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
