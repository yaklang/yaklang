package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type BlueprintFieldKind int

const (
	// method: static normal magic
	BluePrintStaticMethod BlueprintFieldKind = iota
	BluePrintNormalMethod
	BluePrintMagicMethod

	// member: normal const static
	BluePrintNormalMember
	BluePrintConstMember
	BluePrintStaticMember
)

type BlueprintModifier int

const (
	NoneModifier BlueprintModifier = 1 << iota
	Static
	Public
	Protected
	Private
	Abstract
	Final
	Readonly
)

type BlueprintKind int

const (
	BlueprintNone BlueprintKind = iota
	BlueprintClass
	BlueprintInterface
	BlueprintEnum
	BlueprintStruct
)

type BlueprintRelationKind string

const (
	BlueprintRelationParents   BlueprintRelationKind = "__parents__"
	BlueprintRelationSuper                           = "__super__"
	BlueprintRelationInterface                       = "__interface__"

	BlueprintRelationChildren = "__children__"
	BlueprintRelationSub      = "__sub__"
	BlueprintRelationImpl     = "__impl__"
)

func (b BlueprintRelationKind) getRelativeRelation() BlueprintRelationKind {
	switch b {
	case BlueprintRelationParents:
		return BlueprintRelationChildren
	case BlueprintRelationSuper:
		return BlueprintRelationSub
	case BlueprintRelationInterface:
		return BlueprintRelationImpl
	case BlueprintRelationChildren:
		return BlueprintRelationParents
	case BlueprintRelationSub:
		return BlueprintRelationSuper
	case BlueprintRelationImpl:
		return BlueprintRelationInterface
	}
	return ""
}

// Blueprint is a class blueprint, it is used to create a new class
type Blueprint struct {
	Name string

	Kind         BlueprintKind
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

	ParentBlueprints    []*Blueprint // ParentBlueprints All classes, including interfaces and parent classes
	SuperBlueprints     []*Blueprint
	InterfaceBlueprints []*Blueprint
	// full Type Name
	fullTypeName []string

	// lazy
	lazyBuilder
}

func NewBlueprint(name string) *Blueprint {
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

// ======================= class blueprint=======================
func (c *Blueprint) addParentBlueprintEx(parent *Blueprint, relation BlueprintRelationKind) {
	if parent == nil || c == nil {
		return
	}
	if parent == nil {
		return
	}

	c.setBlueprintRelation(parent, relation)
	if relation == BlueprintRelationParents {
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
}

func (c *Blueprint) setBlueprintRelation(parent *Blueprint, relation BlueprintRelationKind) {
	if parent == nil || c == nil {
		return
	}
	switch relation {
	case BlueprintRelationParents:
		c.ParentBlueprints = append(c.ParentBlueprints, parent)
	case BlueprintRelationSuper:
		c.SuperBlueprints = append(c.SuperBlueprints, parent)
	case BlueprintRelationInterface:
		c.InterfaceBlueprints = append(c.InterfaceBlueprints, parent)
	default:
		log.Errorf("BUG!: add parent blueprint error: unknown relation %v", relation)
		return
	}
	c.storeBlueprintRelation(parent, relation)
}

func (c *Blueprint) AddParentBlueprint(parent *Blueprint) {
	c.addParentBlueprintEx(parent, BlueprintRelationParents)
}

func (c *Blueprint) AddSuperBlueprint(parent *Blueprint) {
	c.addParentBlueprintEx(parent, BlueprintRelationSuper)
}

func (c *Blueprint) AddInterfaceBlueprint(b *Blueprint) {
	c.addParentBlueprintEx(b, BlueprintRelationInterface)
}

// GetSuperBlueprint 获取父类，用于单继承
func (c *Blueprint) GetSuperBlueprint() *Blueprint {
	if c == nil {
		return nil
	}
	if c.SuperBlueprints == nil || len(c.SuperBlueprints) == 0 {
		return nil
	}
	return c.SuperBlueprints[0]
}

// GetSuperBlueprints 获取父类，用于多继承
func (c *Blueprint) GetSuperBlueprints() []*Blueprint {
	if c == nil {
		return nil
	}
	return c.SuperBlueprints
}

func (c *Blueprint) GetInterfaceBlueprint() []*Blueprint {
	if c == nil {
		return nil
	}
	return c.InterfaceBlueprints
}

func (c *Blueprint) CheckExtendBy(kls string) bool {
	for _, class := range c.ParentBlueprints {
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
		for _, class := range c.ParentBlueprints {
			// if parent class can get this field, just return true
			if ex := class.getFieldWithParent(get); ex {
				return true
			}
		}
	}
	// not found this field
	return false
}

// storeField store static in global container
func (c *Blueprint) storeField(name string, val Value, _type BlueprintFieldKind) {
	if utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	createVariable := func(builder *FunctionBuilder, variable *Variable) {
		builder.AssignVariable(variable, val)
	}
	builder := c._container.GetFunc().builder
	createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name)))
}

func (c *Blueprint) storeBlueprintRelation(other *Blueprint, relation BlueprintRelationKind) {
	if utils.IsNil(c) || utils.IsNil(c._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}
	if utils.IsNil(other) || utils.IsNil(other._container) || utils.IsNil(c._container.GetFunc()) {
		return
	}

	builder := c._container.GetFunc().builder
	val := builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(string(relation)))
	builder.AssignVariable(val, other._container)
	// set relative relation
	otherBuilder := other._container.GetFunc().builder
	relativeRela := relation.getRelativeRelation()
	if string(relativeRela) == "" {
		return
	}
	otherVal := otherBuilder.CreateMemberCallVariable(other._container, otherBuilder.EmitConstInst(string(relativeRela)))
	otherBuilder.AssignVariable(otherVal, c._container)
}

func (b *Blueprint) InitializeWithContainer(con *Make) error {
	if b._container != nil {
		return utils.Errorf("the container is already initialized id:(%v)", b._container.GetId())
	}
	b._container = con
	return nil
}

func (b *Blueprint) Container() Value {
	return b._container
}
