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
	targetFuncCallee := target.GetFunc()

	fixCallVariadic := func(args []Value, callee *Function) []Value {
		if utils.IsNil(callee) || !callee.hasEllipsis || callee.ParamLength <= 0 || args == nil {
			return args
		}

		fixedCount := callee.ParamLength - 1 // 最后一个是 variadic
		if fixedCount > len(args) {
			fixedCount = len(args)
		}

		// 固定参数部分
		newArgs := append([]Value{}, args[:fixedCount]...)

		// variadic 部分
		variadicArgs := make([]Value, 0)
		for i := fixedCount; i < len(args); i++ {
			variadicArgs = append(variadicArgs, args[i])
		}

		if len(variadicArgs) == 0 { // 如果没有可变参数那就不要塞一个空的make进去了
			return args
		}

		// 打包成 slice
		obj := f.InterfaceAddFieldBuild(len(variadicArgs),
			func(i int) Value { return f.EmitConstInstPlaceholder(i) },
			func(i int) Value { return variadicArgs[i] },
		)
		obj.GetType().(*ObjectType).Kind = SliceTypeKind
		newArgs = append(newArgs, obj)

		return newArgs
	}

	args = fixCallVariadic(args, targetFuncCallee)

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
		log.Infof("ab")
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
		log.Infof("b")
	}
	if target.GetType() == nil {
		log.Infof("aa")
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
		return
	}

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
