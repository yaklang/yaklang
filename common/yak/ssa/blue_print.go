package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type BluePrintFieldKind int

const (
	// method: static normal magic
	BluePrintStaticMethod BluePrintFieldKind = iota
	BluePrintNormalMethod
	BluePrintMagicMethod

	// member: normal const static
	BluePrintNormalMember
	BluePrintConstMember
	BluePrintStaticMember
)

type ClassModifier int

const (
	NoneModifier ClassModifier = 1 << iota
	Static
	Public
	Protected
	Private
	Abstract
	Final
	Readonly
)

// ClassBluePrint is a class blue print, it is used to create a new class
type ClassBluePrint struct {
	Name string

	NormalMethod map[string]*Function
	StaticMethod map[string]*Function
	MagicMethod  map[BluePrintMagicMethodKind]*Function

	NormalMember map[string]Value
	StaticMember map[string]*Phi
	ConstValue   map[string]Value

	CallBack []func()

	// magic method
	Constructor Value
	Destructor  Value

	// _container is an inner ssa.Value
	// the container can ref to the class member
	// _container in this scope
	_container Value

	GeneralPhi       func(string) *Phi
	GeneralUndefined func(string) *Undefined

	ParentClass []*ClassBluePrint
	// full Type Name
	fullTypeName []string

	// lazy
	lazyBuilder
}

func NewClassBluePrint(name string) *ClassBluePrint {
	class := &ClassBluePrint{
		Name:         name,
		NormalMember: make(map[string]Value),
		StaticMember: make(map[string]*Phi),
		ConstValue:   make(map[string]Value),

		NormalMethod: make(map[string]*Function),
		StaticMethod: make(map[string]*Function),
		MagicMethod:  make(map[BluePrintMagicMethodKind]*Function),

		fullTypeName: make([]string, 0),
	}
	return class
}

// ======================= class blue print
// AddParentClass is used to add a parent class to the class,
func (c *ClassBluePrint) AddParentClass(parent *ClassBluePrint) {
	if parent == nil {
		return
	}
	c.ParentClass = append(c.ParentClass, parent)
	for name, f := range parent.NormalMethod {
		c.RegisterNormalMethod(name, f)
	}
	for name, f := range parent.StaticMethod {
		c.RegisterStaticMethod(name, f)
	}
	for name, f := range parent.MagicMethod {
		c.RegisterMagicMethod(name, f)
	}
	for name, value := range parent.NormalMember {
		c.RegisterNormalMember(name, value)
	}
	for name, value := range parent.StaticMember {
		c.RegisterStaticMember(name, value)
	}
	for name, value := range parent.ConstValue {
		c.RegisterConstMember(name, value)
	}
}
func (c *ClassBluePrint) CheckExtendBy(kls string) bool {
	for _, class := range c.ParentClass {
		if strings.EqualFold(class.Name, kls) {
			return true
		}
	}
	return false
}

func (c *ClassBluePrint) getFieldWithParent(get func(bluePrint *ClassBluePrint) bool) bool {
	// if current class can get this field, just return true
	if ok := get(c); ok {
		return true
	} else {
		// if current class can't get this field, then check the parent class
		for _, class := range c.ParentClass {
			// if parent class can get this field, just return true
			if ex := class.getFieldWithParent(get); ex {
				return true
			}
		}
	}
	// not found this field
	return false
}

// storeInContainer store static in global container
func (c *ClassBluePrint) storeInContainer(name string, val Value, _type BluePrintFieldKind) {
	if utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	createVariable := func(builder *FunctionBuilder, variable *Variable) {
		builder.AssignVariable(variable, val)
	}
	switch _type {
	case BluePrintStaticMethod, BluePrintStaticMember:
	default:
		builder := c._container.GetFunc().builder
		createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name)))
	}
}
func (b *ClassBluePrint) InitializeWithContainer(con *Make) error {
	if b._container != nil {
		return utils.Errorf("the container is already initialized id:(%v)", b._container.GetId())
	}
	b._container = con
	return nil
}
func (b *ClassBluePrint) GetClassContainer() Value {
	return b._container
}

func (c *ClassBluePrint) BuildConstructorAndDestructor() {
	for _, p := range c.ParentClass {
		p.BuildConstructorAndDestructor()
	}

	if c.Constructor != nil {
		c.Constructor.GetFunc()
		if function, b := ToFunction(c.Constructor); b {
			function.Build()
		}
	}
	for _, m := range c.Method {
		m.Build()
	}
	for _, function := range c.StaticMethod {
		function.Build()
	}
}
