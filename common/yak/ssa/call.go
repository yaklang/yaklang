package ssa

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"

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
		anValue:             NewValue(),
		Method:              target,
		Args:                args,
		Binding:             binding,
		Async:               false,
		Unpack:              false,
		IsDropError:         false,
		IsEllipsis:          false,
		SideEffectValue:     map[string]Value{},
		MarkParameterMember: make(map[string]Value),
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
	c.handleMoreParameterMember()
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
func (c *Call) handleMoreParameterMember() {
	fun := c.GetFunc()
	builder := fun.builder
	typ := c.Method.GetType()
	functionType, ok := ToFunctionType(typ)
	if !ok {
		log.Errorf("method type not function Type")
		return
	}
	for _, member := range functionType.MarkParameterMember {
		val, ok := member.Get(c, builder)
		if !ok {
			log.Errorf("[BUG]: parameter member get fail")
			continue
		}
		//copy ud
		for _, user := range member.GetUsers() {
			val.AddUser(user)
		}

		//绑定多级实参
		c.MarkParameterMember[member.verboseName] = val
	}
}

// handler if method, set object for first argument
func (c *Call) handleCalleeFunction() {
	function := c.GetFunc()
	builder := function.builder

	// get function type
	funcTyp, ok := ToFunctionType(c.Method.GetType())
	if !ok { // for Test_SideEffect_Double_more
		if param, ok := ToParameter(c.Method); ok {
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
				key := p.MemberCallKey
				object, ok := p.Get(c, builder)
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
				val = builder.ReadMemberCallValue(object, key)
				val.AddUser(c)
				for _, i := range p.GetUsers() {
					val.AddUser(i)
				}
				c.ArgMember = append(c.ArgMember, val)
			}
			break
		}
	}

	if builder.isBindLanguage() {
		handleSideEffectBind(c, funcTyp)
	} else {
		handleSideEffect(c, funcTyp)
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
		handleError := func() {
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
					return
				}
			}
			// other code will mark error in function call-site
			c.NewError(Error, SSATAG, BindingNotFoundInCall(fv.GetName()))
		}

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
