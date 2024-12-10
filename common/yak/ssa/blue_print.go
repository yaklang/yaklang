package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
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

	NormalMethod map[string]Functions
	StaticMethod map[string]Functions
	MagicMethod  map[BlueprintMagicMethodKind]Functions

	NormalMember map[string]Value
	StaticMember map[string]Value
	ConstValue   map[string]Value

	CallBack []func()

	// magic method
	Constructor Value
	Destructor  Value

	// _container is an inner ssa.Valueorigin cls container
	_container Value

	GenerateFunction func(string) *Function

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

		NormalMethod: make(map[string]Functions),
		StaticMethod: make(map[string]Functions),
		MagicMethod:  make(map[BlueprintMagicMethodKind]Functions),

		fullTypeName: make([]string, 0),
	}
	return class
}

func apply[T string | BlueprintMagicMethodKind](methods map[T]Functions, applyMethod func(name T, function *Function)) {
	for name, functions := range methods {
		for _, function := range functions {
			applyMethod(name, function)
		}
	}
}

// ======================= class blue print
// AddParentClass is used to add a parent class to the class,
func (c *Blueprint) AddParentClass(parent *Blueprint) {
	if parent == nil {
		return
	}
	c.ParentClass = append(c.ParentClass, parent)
	apply(parent.NormalMethod, func(name string, function *Function) {
		c.RegisterNormalMethod(name, function, false)
	})
	apply(parent.StaticMethod, func(name string, function *Function) {
		c.RegisterStaticMethod(name, function)
	})
	apply(parent.MagicMethod, func(name BlueprintMagicMethodKind, function *Function) {
		c.RegisterMagicMethod(name, function)
	})
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

func (c *Blueprint) IsParent(p Type) bool {
	if typ, b := ToClassBluePrintType(p); !b {
		return false
	} else {
		for _, class := range typ.ParentClass {
			if c == class {
				return true
			}
		}
	}
	return false
}
