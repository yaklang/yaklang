package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type bluePrintFieldKind int

const (
	// method: static normal magic
	BluePrintStaticMethod bluePrintFieldKind = iota
	BluePrintNormalMethod
	BluePrintMagicMethod

	// member: normal const static
	BluePrintNormalMember
	BluePrintConstMember
	BluePrintStaticMember

	// relation kind
	BlueprintRelationShip
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

type blueprintKind int

const (
	BlueprintNone blueprintKind = iota
	BlueprintClass
	BlueprintInterface
	BlueprintEnum
	BlueprintStruct
)

// type blueprintRelation struct {
// 	// use by parent
// 	children_variable string
// 	// use by children
// 	parent_variable string
// }

var (
	children_variable = "children"
	parent_variable   = "parents"
	// is this needed??
	// BlueprintRelationNormal     = blueprintRelation{"children", "parents"} // all relation should set this
	// BlueprintRelationExtends    = blueprintRelation{"sub", "supper"}       // class extends
	// BlueprintRelationImplements = blueprintRelation{"impl", "interface"}   // interface implements
	// BlueprintRelationEmbed      = blueprintRelation{"embed", "embedded"}   // golang struct embed
	// BlueprintRelationPermits permits // java
)

// Blueprint is a class blueprint, it is used to create a new class
type Blueprint struct {
	Name string
	kind blueprintKind

	NormalMethod map[string]*Function
	StaticMethod map[string]*Function
	MagicMethod  map[BlueprintMagicMethodKind]*Function

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
		MagicMethod:  make(map[BlueprintMagicMethodKind]*Function),

		fullTypeName: make([]string, 0),
	}
	return class
}

// ======================= class blue print
// AddParentBlueprint is used to add a parent class to the class,
func (c *Blueprint) AddParentBlueprint(parent *Blueprint) {
	if parent == nil {
		return
	}
	c.ParentClass = append(c.ParentClass, parent)

	// handler blueprint relation
	c.storeInContainer(parent_variable, parent.GetClassContainer(), BlueprintRelationShip)
	parent.storeInContainer(children_variable, c.GetClassContainer(), BlueprintRelationShip)

	// handler member and method
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
func (c *Blueprint) storeInContainer(name string, val Value, _type bluePrintFieldKind) {
	if utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	builder := c._container.GetFunc().builder
	variable := builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name))
	builder.AssignVariable(variable, val)
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

	if c.Constructor != nil {
		c.Constructor.GetFunc()
		if function, b := ToFunction(c.Constructor); b {
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
