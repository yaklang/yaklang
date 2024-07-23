package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type method struct {
	function *Function
	index    int
}

type BluePrintMember struct {
	Value Value
	Type  Type
}

// ClassBluePrint is a class blue print, it is used to create a new class
type ClassBluePrint struct {
	Name string

	Method       map[string]*Function
	StaticMethod map[string]*Function

	NormalMember map[string]*BluePrintMember
	StaticMember map[string]Value
	ConstMember  map[string]Value

	CallBack []func()

	// magic method
	Copy        Value
	Constructor Value
	Destructor  Value

	// _container is an inner ssa.Value
	// the container can ref to the class member
	_container Value

	ParentClass []*ClassBluePrint
}

func (b *ClassBluePrint) InitializeWithContainer(con *Make) error {
	if b._container != nil {
		return utils.Errorf("the container is already initialized id:(%v)", b._container.GetId())
	}
	b._container = con
	return nil
}

func NewClassBluePrint() *ClassBluePrint {
	class := &ClassBluePrint{
		NormalMember: make(map[string]*BluePrintMember),
		StaticMember: make(map[string]Value),

		Method:       make(map[string]*Function),
		StaticMethod: make(map[string]*Function),
	}

	return class
}

var _ Type = (*ClassBluePrint)(nil)

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
func (c *ClassBluePrint) SetMethod(m map[string]*Function) {
	c.Method = m
}
func (c *ClassBluePrint) AddMethod(key string, fun *Function) {
	if c._container != nil {
		// set the container ref key to the method
		log.Infof("bind %v.%v to function: %v", c.Name, key, fun.name)
		funcContainsklass := c._container.GetFunc()
		if funcContainsklass != nil && funcContainsklass.builder != nil {
			builder := funcContainsklass.builder
			variable := builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(key))
			builder.AssignVariable(variable, fun)
		} else {
			log.Warnf("bind %v.%v failed, reason: class's builder (from source is missed)", c.Name, key)
		}
	} else {
		log.Warnf("class %v's ref container is nil", c.Name)
	}
	fun.SetMethod(true, c)
	c.Method[key] = fun
}
func (c *ClassBluePrint) GetMethod() map[string]*Function {
	return c.Method
}
