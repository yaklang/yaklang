package ssa

import (
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

// Blueprint is a class blueprint, it is used to create a new class
type Blueprint struct {
	Name string

	NormalMethod map[string]*Function
	StaticMethod map[string]*Function
	MagicMethod  map[BlueprintMagicMethodKind]Value

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

	ParentClass []*Blueprint
	// full Type Name
	fullTypeName []string

	// lazy
	lazyBuilder
}

func NewClassBluePrint(name string) *Blueprint {
	class := &Blueprint{
		Name:         name,
		NormalMember: make(map[string]Value),
		StaticMember: make(map[string]Value),
		ConstValue:   make(map[string]Value),

		NormalMethod: make(map[string]*Function),
		StaticMethod: make(map[string]*Function),
		MagicMethod:  make(map[BlueprintMagicMethodKind]Value),

		fullTypeName: make([]string, 0),
	}
	return class
}

// ======================= class blue print
// AddParentClass is used to add a parent class to the class,
func (c *Blueprint) AddParentClass(parent *Blueprint) {
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
func (c *Blueprint) CheckExtendBy(kls string) bool {
	for _, class := range c.ParentClass {
		if strings.EqualFold(class.Name, kls) {
			return true
		}
	}
	return false
}

func (c *Blueprint) getFieldWithParent(get func(bluePrint *Blueprint) bool) bool {
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
func (c *Blueprint) storeInContainer(name string, val Value, _type BluePrintFieldKind) {
	if utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	createVariable := func(builder *FunctionBuilder, variable *Variable) {
		builder.AssignVariable(variable, val)
	}
	builder := c._container.GetFunc().builder
	createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name)))
}
func (b *Blueprint) InitializeWithContainer(con *Make) error {
	if b._container != nil {
		return utils.Errorf("the container is already initialized id:(%v)", b._container.GetId())
	}
	b._container = con
	return nil
}
func (b *Blueprint) GetClassContainer() Value {
	return b._container
}

func (c *Blueprint) BuildConstructorAndDestructor() {
	for _, p := range c.ParentClass {
		p.BuildConstructorAndDestructor()
	}

	for _, value := range c.MagicMethod {
		if function, b := ToFunction(value); b {
			function.Build()
		}

	}
	for _, m := range c.NormalMethod {
		m.Build()
	}
	for _, function := range c.StaticMethod {
		function.Build()
	}
}
