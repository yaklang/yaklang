package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

// ClassBluePrint is a class blue print, it is used to create a new class
type ClassBluePrint struct {
	This Value
	*ObjectType
	// this field, is function pin to this object,
	// when set type to a new object, it will be set to the new object
	MarkedField map[Value]*FunctionType // key -> value
	NormalField map[Value]Type          // key -> value

	// Static Member
	NormalMember map[string]Value
	StaticMember map[string]Value
}

func NewClassBluePrint() *ClassBluePrint {
	class := &ClassBluePrint{
		This:         nil,
		ObjectType:   nil,
		MarkedField:  make(map[Value]*FunctionType),
		NormalField:  make(map[Value]Type),
		NormalMember: make(map[string]Value),
		StaticMember: make(map[string]Value),
	}
	return class
}

func (c *ClassBluePrint) SetThis(v Value) {
	c.This = v
}
func (c *ClassBluePrint) SetObjectType(t *ObjectType) {
	c.ObjectType = t
}

var _ Type = (*ClassBluePrint)(nil)

// ParseClassBluePrint  parse get classBluePrint if the ObjectType is a ClassFactor
func ParseClassBluePrint(this Value, objectTyp *ObjectType) (ret Type) {
	ret = objectTyp

	if !this.IsObject() {
		return
	}
	blue := NewClassBluePrint()
	blue.SetThis(this)
	blue.SetObjectType(objectTyp)

	for key, member := range this.GetAllMember() {
		// if not function , just append this field to normal field
		typ := member.GetType()
		if typ.GetTypeKind() != FunctionTypeKind {
			blue.NormalField[key] = typ
			continue
		}

		funcType, ok := ToFunctionType(typ)
		if !ok {
			log.Errorf("ParseClassBluePrint: %s is not a function type but is FunctionTypeKind", typ)
			continue
		}

		has := false
		for _, fv := range funcType.ParameterValue {
			if fv.GetDefault() == this {
				has = true
			}
		}

		if has {
			blue.MarkedField[key] = funcType
		} else {
			blue.NormalField[key] = typ
		}
	}

	if len(blue.MarkedField) != 0 {
		return blue
	}

	return
}

func (c *ClassBluePrint) Apply(obj Value) Type {
	builder := obj.GetFunc().builder
	_ = builder
	this := c.This

	objTyp := NewObjectType()

	for key, typ := range c.NormalField {
		objTyp.AddField(key, typ)
	}

	for key, funTyp := range c.MarkedField {
		new := funTyp.Copy()

		for index, para := range new.ParameterValue {
			if v := para.GetDefault(); v == this {
				newPara := para.Copy()
				newPara.SetDefault(obj)
				new.ParameterValue[index] = newPara
			}
		}

		objTyp.AddField(key, new)
	}

	return objTyp
}

/// ============= implement type interface

func (c *ClassBluePrint) String() string {
	str := fmt.Sprintf("ClassBluePrint: %s", c.This.GetName())
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
func (c *ClassBluePrint) SetMethod(map[string]*FunctionType) {
}
func (c *ClassBluePrint) AddMethod(string, *FunctionType) {
}
func (c *ClassBluePrint) GetMethod() map[string]*FunctionType {
	return nil
}
