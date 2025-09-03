package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type BlueprintFieldKind int

// 定义最大继承深度
const MaxInheritanceDepth = 100

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
	id   int64
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

	Range *memedit.Range

	ParentBlueprints    []*Blueprint
	InterfaceBlueprints []*Blueprint
	// full Type Name
	fullTypeName []string

	// lazy
	LazyBuilder
}

func (b *Blueprint) GetId() int64 {
	return b.id
}
func (b *Blueprint) SetId(id int64) {
	b.id = id
}

func NewBlueprint(name string) *Blueprint {
	class := &Blueprint{
		id:           -1,
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

// HasCircularDependency 检查两个蓝图之间是否存在循环依赖
func HasCircularDependency(b1, b2 *Blueprint) bool {
	if b1 == nil || b2 == nil {
		return false
	}

	// 检查 b1 的继承链中是否包含 b2
	// 每次独立检查时都初始化新的 visited 集合和从0开始的深度
	visited1 := make(map[*Blueprint]bool)
	if b1.inheritsFromWithVisited(b2, visited1, 0) {
		return true
	}
	visited1 = nil

	// 检查 b2 的继承链中是否包含 b1
	visited2 := make(map[*Blueprint]bool)
	if b2.inheritsFromWithVisited(b1, visited2, 0) {
		return true
	}

	return false
}

// inheritsFromWithVisited 递归检查当前蓝图是否继承自目标蓝图，
// 并使用 visited 集合避免重复访问，同时进行深度控制。
func (c *Blueprint) inheritsFromWithVisited(target *Blueprint, visited map[*Blueprint]bool, currentDepth int) bool {
	// 深度控制：如果超过最大允许深度，则认为无法找到或存在过深的继承链，直接返回 false
	if currentDepth > MaxInheritanceDepth {
		// 可以在这里选择记录一个警告，表示继承链过深
		log.Warnf("Inheritance chain for blueprint '%s' exceeded max depth of %d. Potential issue or too deep hierarchy.", c.Name, MaxInheritanceDepth)
		return false
	}

	// 如果当前蓝图已经访问过，直接返回 false，避免无限递归和重复计算
	if visited[c] {
		return false
	}
	visited[c] = true // 标记当前蓝图为已访问

	// 如果当前蓝图就是目标蓝图，说明存在继承关系
	if c == target {
		return true
	}

	// 遍历父蓝图，深度递增
	for _, parent := range c.ParentBlueprints {
		if parent.inheritsFromWithVisited(target, visited, currentDepth+1) {
			return true
		}
	}

	// 所有路径都探索完毕，没有找到目标蓝图
	return false
}

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

	// 使用新的循环依赖检查函数
	if HasCircularDependency(c, parent) {
		log.Errorf("BUG!: add parent blueprint error: loop. blueprint name: %v, parent name: %v", c.Name, parent.Name)
		return
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

func (c *Blueprint) getFieldWithParent(get func(bluePrint *Blueprint) bool, recursiveLevel ...int) bool {
	currentRecursiveLevel := 0
	if recursiveLevel != nil {
		currentRecursiveLevel = recursiveLevel[0]
	}
	if currentRecursiveLevel > MaxInheritanceDepth {
		log.Error("failed to get field from parents, inherit chain too long")
		return false
	}
	// if current class can get this field, just return true
	if ok := get(c); ok {
		return true
	} else {
		// if current class can't get this field, then check the parent class
		for _, class := range c.ParentBlueprints {
			// if parent class can get this field, just return true
			if ex := class.getFieldWithParent(get, currentRecursiveLevel); ex {
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
	createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInstPlaceholder(name)))
}

func (b *Blueprint) InitializeWithContainer(con Value) error {
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
