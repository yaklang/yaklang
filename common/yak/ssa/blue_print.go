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

type BlueprintKind string

const (
	BlueprintNone      BlueprintKind = "none"
	BlueprintClass                   = "class"
	BlueprintInterface               = "interface"

	//BlueprintObject for object, like new Blueprint
	BlueprintObject = "object"
)

func ValidBlueprintKind(str string) BlueprintKind {
	switch str {
	case "none":
		return BlueprintNone
	case "class":
		return BlueprintClass
	case "interface":
		return BlueprintInterface
	case "object":
		return BlueprintObject
	default:
		return BlueprintNone
	}
}

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

	ParentBlueprints    []*Blueprint
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

func (c *Blueprint) GetInterfaceBlueprint() []*Blueprint {
	if c == nil {
		return nil
	}
	return c.InterfaceBlueprints
}

func (c *Blueprint) GetParentBlueprint() []*Blueprint {
	if c == nil {
		return nil
	}
	return c.ParentBlueprints
}

// GetSuperBlueprint only get the first parent blueprint, using for single inheritance
func (c *Blueprint) GetSuperBlueprint() *Blueprint {
	if c == nil {
		return nil
	}
	if len(c.ParentBlueprints) > 0 {
		return c.ParentBlueprints[0]
	}
	return nil
}

func (c *Blueprint) GetAllParentsBlueprint() []*Blueprint {
	if c == nil {
		return nil
	}
	// 层序遍历
	visited := make(map[*Blueprint]bool)
	var allParents []*Blueprint
	queue := c.GetParentBlueprint()

	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if parent == nil || visited[parent] {
			continue
		}
		if visited[parent] {
			continue
		}

		visited[parent] = true
		allParents = append(allParents, parent)
		queue = append(queue, parent.GetParentBlueprint()...)
	}

	return allParents
}

func (c *Blueprint) GetAllInterfaceBlueprints() []*Blueprint {
	if c == nil {
		return nil
	}
	// 层序遍历
	visited := make(map[*Blueprint]bool)
	var allParents []*Blueprint
	queue := c.GetInterfaceBlueprint()

	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if parent == nil || visited[parent] {
			continue
		}
		if visited[parent] {
			continue
		}

		visited[parent] = true
		allParents = append(allParents, parent)
		queue = append(queue, parent.GetInterfaceBlueprint()...)
	}

	return allParents
}

func (c *Blueprint) GetRootParentBlueprints() []*Blueprint {
	if c == nil {
		return nil
	}

	var roots []*Blueprint
	visited := make(map[*Blueprint]bool)
	var dfs func(*Blueprint)

	dfs = func(node *Blueprint) {
		if node == nil || visited[node] {
			return
		}
		visited[node] = true

		parents := node.GetParentBlueprint()
		if len(parents) == 0 {
			roots = append(roots, node)
			return
		}

		for _, parent := range parents {
			dfs(parent)
		}
	}

	dfs(c)
	return roots
}

func (c *Blueprint) GetRootInterfaceBlueprint() []*Blueprint {
	if c == nil {
		return nil
	}

	var roots []*Blueprint
	visited := make(map[*Blueprint]bool)
	var dfs func(*Blueprint)

	dfs = func(node *Blueprint) {
		if node == nil || visited[node] {
			return
		}
		visited[node] = true

		parents := node.GetInterfaceBlueprint()
		if len(parents) == 0 {
			roots = append(roots, node)
			return
		}

		for _, parent := range parents {
			dfs(parent)
		}
	}

	dfs(c)
	return roots
}

func (c *Blueprint) CheckExtendedBy(parentBlueprint *Blueprint) bool {
	for _, blueprint := range c.ParentBlueprints {
		if blueprint == parentBlueprint {
			return true
		}
		return blueprint.CheckExtendedBy(parentBlueprint)
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
	createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name, true)))
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
