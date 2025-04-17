package ssa

import (
	"github.com/yaklang/yaklang/common/log"
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

	//BlueprintObject for object, like new Blueprint
	BlueprintObject
)

type BlueprintRelationKind string

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
		Kind:         BlueprintNone,
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
func (c *Blueprint) AddParentBlueprint(parent *Blueprint) {
	c.addParentBlueprintEx(parent, BlueprintRelationParents)
}

func (c *Blueprint) AddInterfaceBlueprint(b *Blueprint) {
	c.addParentBlueprintEx(b, BlueprintRelationInterface)
}

func (c *Blueprint) addParentBlueprintEx(parent *Blueprint, relation BlueprintRelationKind) {
	if parent == nil || c == nil {
		return
	}
	if relation == BlueprintRelationParents {
		isExist := false
		c.getFieldWithParent(func(bluePrint *Blueprint) bool {
			if bluePrint == parent {
				isExist = true
				return true
			}
			return false
		})
		if !isExist {
			parent.getFieldWithParent(func(bluePrint *Blueprint) bool {
				if bluePrint == c {
					isExist = true
					return true
				}
				return false
			})
		}
		// check loop
		if isExist {
			log.Errorf("BUG!: add parent blueprint error: loop. blueprint name: %v, parent name: %v", c.Name, parent.Name)
			return
		}
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

// GetSuperBlueprint 获取父类，用于单继承
func (c *Blueprint) GetSuperBlueprint() *Blueprint {
	if c == nil {
		return nil
	}
	if len(c.SuperBlueprints) == 0 {
		return nil
	}
	ret := c.SuperBlueprints[0]
	ret.Build()
	return ret
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

func (c *Blueprint) CheckExtendBy(parentBlueprint *Blueprint) bool {
	for _, blueprint := range c.SuperBlueprints {
		if blueprint == parentBlueprint {
			return true
		}
		return blueprint.CheckExtendBy(parentBlueprint)
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

	container := c._container
	builder := container.GetFunc().builder
	createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name)))
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

func (c *Blueprint) SetKind(kind BlueprintKind) {
	if c == nil {
		return
	}
	c.Kind = kind
}
