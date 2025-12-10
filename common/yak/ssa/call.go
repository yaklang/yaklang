package ssa

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func NewCall(target Value, args Values, binding map[string]Value, block *BasicBlock) *Call {
	bind := make(map[string]int64, len(binding))
	for name, value := range binding {
		bind[name] = value.GetId()
	}

	c := &Call{
		anValue:         NewValue(),
		Method:          target.GetId(),
		Args:            args.GetIds(),
		Binding:         bind,
		Async:           false,
		Unpack:          false,
		IsDropError:     false,
		IsEllipsis:      false,
		SideEffectValue: map[string]int64{},
	}
	if c.Method <= 0 {
		log.Errorf("NewCall: method id is %d, target: %s", c.Method, target.String())
	}
	return c
}

func (f *FunctionBuilder) NewCall(target Value, args []Value) *Call {
	// 创建 Call 指令
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
	c.handleMethod()
	fixupUseChain(c)
	return c
}

// handleMethod handle method for call,this call have more type,need handle this type
func (c *Call) handleMethod() {
	callMethod, ok := c.GetValueById(c.Method)
	if !ok || callMethod == nil {
		return
	}
	/*
		handle weakLanguage call
	*/
	builder := c.GetFunc().builder
	var check func(Value)
	weakLanguage := builder.IsSupportConstMethod()
	tmp := make(map[int64]struct{})
	PointFunc := func(fun Value, method Value) {
		Point(fun, method)
	}
	check = func(method Value) {
		_, exist := tmp[method.GetId()]
		if exist {
			return
		}
		tmp[method.GetId()] = struct{}{}
		switch ret := method.(type) {
		case *ConstInst:
			/*
				from <string> to getFunc
			*/
			if weakLanguage {
				if newMethod, ok := builder.GetFunc(method.String(), ""); ok {
					PointFunc(newMethod, callMethod)
				} else {
					log.Errorf("weakLanguage call %s not found", method.String())
					return
				}
			}
		case *SideEffect:
			/*
				from value to getFunc
			*/
			if value, ok := ret.GetValueById(ret.Value); ok && value != nil {
				check(value)
			}
		case *Phi:
			for _, value := range ret.GetValues() {
				check(value)
			}
		}
	}
	if !callMethod.IsExtern() && callMethod.GetOpcode() != SSAOpcodeFunction {
		check(callMethod)
	}
}

func (c *Call) handlerGeneric() {
	newMethod, ok := c.GetValueById(c.Method)
	if !ok || newMethod == nil || !newMethod.IsExtern() || newMethod.GetOpcode() != SSAOpcodeFunction {
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
	// variadicType用于参与泛型cache的key计算
	var variadicType Type

	hasError := false
	for i, id := range c.Args {
		arg, ok := c.GetValueById(id)
		if !ok || arg == nil {
			continue
		}
		index := i
		argTyp := arg.GetType()

		switch {
		case isVariadic && i > fType.ParameterLen-1:
			// 有多个可变长参数
			index = fType.ParameterLen - 1
			variadicType = arg.GetType()
		case isVariadic && i == fType.ParameterLen-1:
			// 可变参数只有一个
			argType := arg.GetType()
			if argType.GetTypeKind() != SliceTypeKind {
				argType = NewSliceType(argType)
			}
			paramsType[i] = argType
			variadicType = argType
		default:
			paramsType[i] = arg.GetType()
		}

		// unpack T...([]T) to T
		if i >= fType.ParameterLen-1 && c.IsEllipsis {
			if argTyp.GetTypeKind() == SliceTypeKind {
				argTyp = argTyp.(*ObjectType).FieldType
			} else if argTyp.GetTypeKind() == BytesTypeKind {
				argTyp = CreateByteType()
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
			instanceTypeMap[typ.String()] = CreateAnyType()
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
	// for variadic function, we need to include the argument types of the variadic parameter
	if isVariadic && variadicType != nil {
		nameBuilder.WriteRune('-')
		nameBuilder.WriteString(variadicType.String())
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
		newMethod = prog.NewFunction(newMethod.GetName())
		newMethod.SetType(newFuncTyp)
		prog.SetCacheExternInstance(name, newMethod)
	} else {
		newMethod = value
	}

	c.Method = newMethod.GetId()
	if c.Method <= 0 {
		log.Errorf("method id is %d, target: %s", c.Method, newMethod.String())
	}
}
func (c *Call) handlerObjectMethod() {
	args := c.Args
	target, ok := c.GetValueById(c.Method)
	if !ok || target == nil {
		return
	}
	// handler "this" in parameter
	AddThis := func(this Value) {
		if len(args) == 0 {
			args = append(args, this.GetId())
		} else {
			args = utils.InsertSliceItem(args, this.GetId(), 0)
		}
		this.AddUser(c)
	}
	if target == nil {
		log.Errorf("target is nil")
	}
	if target.GetType() == nil {
		log.Errorf("target type is nil")
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
	method, ok := c.GetValueById(c.Method)
	if !ok || method == nil {
		return
	}
	funcTyp, ok := ToFunctionType(method.GetType())
	if !ok {
		// For FreeValue Parameter, try to get FunctionType from its Default value
		// This handles cases like mutual recursion where the method is a captured variable
		// Note: Due to circular dependencies in mutual recursion, the Default function
		// may still be building and its Type may not be set yet.
		if param, ok := ToParameter(method); ok && param.IsFreeValue {
			if defVal := param.GetDefault(); defVal != nil {
				if defFunc, isFunc := ToFunction(defVal); isFunc && defFunc.Type != nil {
					funcTyp = defFunc.Type
					ok = true
				}
			}
		}
		if !ok {
			return
		}
	}
	if utils.IsNil(funcTyp.ReturnType) {
		log.Warnf("[ssa.Call.handlerReturnType] skip setting type for call %s: return type is nil", method.GetName())
		return
	}
	// inference call instruction type
	if c.IsDropError {
		if retType, ok := funcTyp.ReturnType.(*ObjectType); ok {
			if retType.Combination && retType.FieldTypes[len(retType.FieldTypes)-1].GetTypeKind() == ErrorTypeKind {
				if len(retType.FieldTypes) == 1 {
					c.SetType(CreateNullType())
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
			c.SetType(CreateNullType())
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
	function := c.GetFunc()
	builder := function.builder

	// get function type
	method, ok := c.GetValueById(c.Method)
	if !ok || method == nil {
		return
	}
	funcTyp, ok := ToFunctionType(method.GetType())
	if !ok { // for Test_SideEffect_Double_more
		if param, ok := ToParameter(method); ok {
			caller := param.GetFunc()
			callee := caller.anValue.GetFunc()
			caller.SideEffects = append(caller.SideEffects, callee.SideEffects...)
		}

		// Handle case where method is a Call instruction (e.g., f1() where f1 = f())
		// Try to get the FunctionType from the call's return value
		if callMethod, ok := ToCall(method); ok {
			// Get the callee of the inner call (e.g., f in f())
			innerCallee, ok := c.GetValueById(callMethod.Method)
			if ok && !utils.IsNil(innerCallee) {
				if innerFunc, ok := ToFunction(innerCallee); ok {
					// Try to get return type from innerFunc.Type first
					if innerFunc.Type != nil {
						if retFuncTyp, ok := ToFunctionType(innerFunc.Type.ReturnType); ok {
							funcTyp = retFuncTyp
							goto handleSideEffects
						}
					}
					// If Type is nil, check the Return statements directly
					// This handles cases where the function type hasn't been fully resolved
					for _, retId := range innerFunc.Return {
						if retInst, ok := innerFunc.GetValueById(retId); ok {
							if ret, ok := ToReturn(retInst); ok {
								for _, resId := range ret.Results {
									if res, ok := innerFunc.GetValueById(resId); ok {
										// Check if the return value is a function with side effects
										if retFuncTyp, ok := ToFunctionType(res.GetType()); ok {
											if len(retFuncTyp.SideEffects) > 0 {
												funcTyp = retFuncTyp
												goto handleSideEffects
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
		return
	}

handleSideEffects:

	{
		recoverBuilder := builder.SetCurrent(c)
		defer func() {
			recoverBuilder()
		}()

		for true {
			if len(c.ArgMember) == len(funcTyp.ParameterMember) {
				break
			}
			for _, p := range funcTyp.ParameterMember {

				objectName := p.ObjectName
				key, ok := p.GetValueById(p.MemberCallKey)
				if !ok || utils.IsNil(key) {
					continue
				}
				object, ok := p.GetActualParam(c)
				if !ok {
					continue
				}
				if utils.IsNil(object) {
					continue
				}

				if res := checkCanMemberCallExist(object, key); !res.exist {
					builder.NewErrorWithPos(Error, SSATAG,
						p.GetRange(),
						ValueNotMember(
							object.GetOpcode(),
							objectName,
							key.String(),
							c.GetRange(),
						),
					)
					c.NewError(Error, SSATAG,
						ValueNotMemberInCall(
							objectName,
							key.String(),
						),
					)
					continue
				}

				var val Value
				if val = builder.ReadMemberCallValueByName(object, key.String()); val == nil {
					if o, ok := object.GetType().(*ObjectType); ok {
						for n, a := range o.AnonymousField {
							if k := a.GetKeybyName(key.String()); k != nil {
								objectt := builder.ReadMemberCallValueByName(object, n)
								if objectt == nil {
									log.Warnf("anonymous object %v not find", n)
									continue
								}
								val = builder.ReadMemberCallValueByName(objectt, k.String())
							}
						}
					}
				}

				if _, ok := ToExternLib(object); ok {
					continue
				}

				if utils.IsNil(val) {
					val = builder.ReadMemberCallValue(object, key)
				}
				val.AddUser(c)
				c.ArgMember = append(c.ArgMember, val.GetId())
			}
			break
		}
	}

	if builder.isBindLanguage() {
		handleSideEffectBind(c, funcTyp)
	} else {
		handleSideEffect(c, funcTyp, false)
	}
	handleSideEffect(c, funcTyp, true)

	// Handle side effects from function arguments
	// When a function with side effects is passed as an argument and will be called
	// inside the callee, we need to propagate its side effects to the call site
	handleArgumentFunctionSideEffect(c, funcTyp)

	// only handler in method call
	if !funcTyp.IsMethod {
		return
	}

	if !method.IsMember() {
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
		c.Binding[name] = val.GetId()
	}

	for _, fv := range fvs {
		handleError := func() {
			// mark error in freeValue.Variable
			// get freeValue
			if variable := fv.GetVariable(fv.GetName()); variable != nil {
				variable.NewError(Error, SSATAG, BindingNotFound(fv.GetName(), c.GetRange()))
			}
			// skip instance function, or `go` with instance function,
			// this function no variable, and code-range of call-site same as function.
			// we don't mark error in call-site.
			method, ok := c.GetValueById(c.Method)
			if ok && method != nil {
				if fun, ok := ToFunction(method); ok {
					if len(fun.GetAllVariables()) == 0 {
						return
					}
				}
			}
			// other code will mark error in function call-site
			c.NewError(Error, SSATAG, BindingNotFoundInCall(fv.GetName()))
		}

		/*
			在call的时候，从call scope中读取freeValue，然后绑定ud关系
		*/
		value := builder.PeekValue(fv.GetName())
		if fv.GetDefault() == nil && value != nil {
			bindAndHandler(fv.GetName(), value)
			continue
		} else if fv.GetDefault() == nil && value == nil {
			handleError()
			continue
		}

		var findVariableCaptured, bindVariableCaptured ssautil.VersionedIF[Value]
		if value != nil {
			if findVariable := value.GetLastVariable(); findVariable != nil {
				findVariableCaptured = findVariable.GetCaptured()
			}
		}
		if fv.GetDefault() != nil {
			if bindVariable := fv.GetDefault().GetLastVariable(); bindVariable != nil {
				bindVariableCaptured = bindVariable.GetCaptured()
			}
		}

		if value != nil && findVariableCaptured == bindVariableCaptured {
			bindAndHandler(fv.GetName(), value)
		} else {
			bindAndHandler(fv.GetName(), fv.GetDefault())
		}

		if _, ok := c.Binding[fv.GetName()]; !ok {
			handleError()
		}
	}
}
