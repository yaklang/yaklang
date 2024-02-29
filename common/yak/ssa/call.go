package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

func NewCall(target Value, args, binding []Value, block *BasicBlock) *Call {
	c := &Call{
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
		Method:        target,
		Args:          args,
		binding:       binding,
		Async:         false,
		Unpack:        false,
		IsDropError:   false,
		IsEllipsis:    false,
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
	c.handlerReturnType()
	c.handleMethod()

	return c
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
}

// handler if method, set object for first argument
func (c *Call) handleMethod() {

	// get function type
	funcTyp, ok := ToFunctionType(c.Method.GetType())
	if !ok {
		return
	}
	{
		builder := c.GetFunc().builder
		recoverBuilder := builder.SetCurrent(c)
		for name, value := range funcTyp.SideEffects {
			variable := builder.CreateVariable(name)
			builder.AssignVariable(variable, value)
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

	// get object
	obj := c.Method.GetObject()
	c.Args = utils.InsertSliceItem(c.Args, obj, 0)
}

func (c *Call) HandleFreeValue(fvs []string) {

	builder := c.GetFunc().builder
	recoverBuilder := builder.SetCurrent(c)
	defer recoverBuilder()

	for _, name := range fvs {
		v := builder.PeekValue(name)

		if v != nil {
			c.binding = append(c.binding, v)
		} else {
			// mark error in freeValue.Variable
			// get freeValue
			fun, ok := ToFunction(c.Method)
			if !ok {
				continue
			}
			freeValue, ok := fun.FreeValues[name]
			if !ok {
				continue
			}
			if variable := freeValue.GetVariable(name); variable != nil {
				variable.NewError(Error, SSATAG, BindingNotFound(name, c.GetRange()))
				if len(fun.GetAllVariables()) != 0 {
					c.NewError(Error, SSATAG, BindingNotFoundInCall(name))
				}
			}
		}
	}
}
