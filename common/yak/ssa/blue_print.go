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
type BluePrint struct {
	Name string

	NormalMethod map[string]*Function
	StaticMethod map[string]*Function
	MagicMethod  map[BluePrintMagicMethodKind]*Function

	NormalMember map[string]Value
	StaticMember map[string]Value
	ConstValue   map[string]Value

	CallBack []func()

	// magic method
	Constructor Value
	Destructor  Value

	// _container is an inner ssa.Valueorigin cls container
	_container Value

	GeneralUndefined func(string) *Undefined

	ParentClass []*BluePrint
	// full Type Name
	fullTypeName []string

	// lazy
	lazyBuilder
}

func NewClassBluePrint(name string) *BluePrint {
	class := &BluePrint{
		Name:         name,
		NormalMember: make(map[string]Value),
		StaticMember: make(map[string]Value),
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
func (c *BluePrint) AddParentClass(parent *BluePrint) {
	if parent == nil {
		return
	}
	c.ParentClass = append(c.ParentClass, parent)
	for name, f := range parent.NormalMethod {
		c.RegisterNormalMethod(name, f, false)
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
func (c *BluePrint) CheckExtendBy(kls string) bool {
	for _, class := range c.ParentClass {
		if strings.EqualFold(class.Name, kls) {
			return true
		}
	}
	return false
}

func (c *BluePrint) getFieldWithParent(get func(bluePrint *BluePrint) bool) bool {
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
func (c *BluePrint) storeInContainer(name string, val Value, _type BluePrintFieldKind) {
	if utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	createVariable := func(builder *FunctionBuilder, variable *Variable) {
		builder.AssignVariable(variable, val)
	}
	builder := c._container.GetFunc().builder
	createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name)))
}
func (b *BluePrint) InitializeWithContainer(con *Make) error {
	if b._container != nil {
		return utils.Errorf("the container is already initialized id:(%v)", b._container.GetId())
	}
	b._container = con
	return nil
}
func (b *BluePrint) GetClassContainer() Value {
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
