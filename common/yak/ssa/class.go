package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

type method struct {
	function *Function
	index    int
}

// ClassBluePrint is a class blue print, it is used to create a new class
type ClassBluePrint struct {
	Name string
	This Value

	NormalMember map[string]Value
	NormalMethod map[string]*method // key -> value

	StaticMember map[string]Value
	StaticMethod map[string]*Function

	// magic method
	Copy        Value
	Constructor Value
	Destructor  Value

	ParentClass []*ClassBluePrint
}

func NewClassBluePrint() *ClassBluePrint {
	class := &ClassBluePrint{
		This:         nil,
		NormalMember: make(map[string]Value),
		NormalMethod: make(map[string]*method),
		StaticMember: make(map[string]Value),
		StaticMethod: make(map[string]*Function),
	}

	return class
}

var _ Type = (*ClassBluePrint)(nil)

func (c *ClassBluePrint) SetThis(v Value) {
	c.This = v
}

// ParseClassBluePrint  parse get classBluePrint if the ObjectType is a ClassFactor
func ParseClassBluePrint(this Value, objectTyp *ObjectType) (ret Type) {
	ret = objectTyp

	if !this.IsObject() {
		return
	}
	blue := NewClassBluePrint()
	blue.SetThis(this)
	// blue.SetObjectType(objectTyp)

	for key, member := range this.GetAllMember() {
		// if not function , just append this field to normal field
		typ := member.GetType()
		if typ.GetTypeKind() != FunctionTypeKind {
			blue.NormalMember[key.String()] = member
			continue
		}

		funcType, ok := ToFunctionType(typ)
		if !ok {
			log.Errorf("ParseClassBluePrint: %s is not a function type but is FunctionTypeKind", typ)
			continue
		}

		has := false
		for index, fv := range funcType.ParameterValue {
			if fv.GetDefault() == this {
				has = true
				blue.NormalMethod[key.String()] = &method{
					function: member.(*Function),
					index:    index,
				}
			}
		}

		if has {
			continue
		}
		blue.NormalMember[key.String()] = member
	}

	if len(blue.NormalMethod) != 0 {
		return blue
	}

	return
}

func (c *ClassBluePrint) Apply(obj Value) Type {
	builder := obj.GetFunc().builder
	_ = builder
	this := c.This
	_ = this

	objTyp := NewObjectType()
	for _, parent := range c.ParentClass {
		parent.Apply(obj)
	}

	for rawKey, member := range c.NormalMember {
		key := NewConst(rawKey)
		log.Infof("apply key: %s, member: %s", key, member)

		objTyp.AddField(key, member.GetType())
		builder.AssignVariable(
			builder.CreateMemberCallVariable(obj, key),
			member,
		)
	}

	for rawKey, method := range c.NormalMethod {
		key := NewConst(rawKey)

		objTyp.AddField(key, method.function.GetType())
		builder.AssignVariable(
			builder.CreateMemberCallVariable(obj, key),
			// function,
			NewClassMethod(method.function, obj, method.index),
		)
	}

	return objTyp
}

/// ============= implement type interface

func (c *ClassBluePrint) String() string {
	str := fmt.Sprintf("ClassBluePrint: %s", c.Name)
	return str
}
func (c *ClassBluePrint) PkgPathString() string {
	return ""
}
func (c *ClassBluePrint) RawString() string {
	return ""
}
func (c *ClassBluePrint) GetTypeKind() TypeKind {
	return ClassBluePrintTypeKind
}
func (c *ClassBluePrint) SetMethod(map[string]*Function) {
}
func (c *ClassBluePrint) AddMethod(string, *Function) {
}
func (c *ClassBluePrint) GetMethod() map[string]*Function {
	return nil
}
