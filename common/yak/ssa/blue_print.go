package ssa

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

type method struct {
	function *Function
	index    int
}

type BluePrintMember struct {
	Value Value
	Type  Type
}
type KlassType int

const (
	staticMethod KlassType = iota
	staticMember
	method_
	member
	magicMethod
	constMember
)

// ClassBluePrint is a class blue print, it is used to create a new class
type ClassBluePrint struct {
	Name string

	Method       map[string]*Function
	StaticMethod map[string]*Function
	MagicMethod  map[string]*Function

	NormalMember  map[string]*BluePrintMember
	StaticMember  map[string]Value
	_shadowMember map[string]Values
	magicMethod   map[string]Value
	ConstValue    map[string]Value

	magicMethodCheck map[string]func(val Value) bool //对于有些魔术方法条件触发进行检查
	CallBack         []func()

	// magic method
	Copy        Value
	Constructor Value
	Destructor  Value

	// _container is an inner ssa.Value
	// the container can ref to the class member
	// _container in this scope
	_container Value

	//_staticContainer static container in globalScope
	_staticContainer Value

	ParentClass []*ClassBluePrint
	// full Type Name
	fullTypeName []string
}

func (c *ClassBluePrint) RegisterMagicMethod(name string, val *Function) {
	c.registerKlassInfo(name, val, magicMethod)
}
func (c *ClassBluePrint) RegisterNormalMember(name string, val Value) {
	c.registerKlassInfo(name, val, member)
}
func (c *ClassBluePrint) RegisterStaticMethod(name string, val *Function) {
	c.registerKlassInfo(name, val, staticMethod)
}
func (c *ClassBluePrint) RegisterNormalMethod(name string, val *Function) {
	c.registerKlassInfo(name, val, method_)
}
func (c *ClassBluePrint) RegisterStaticMember(name string, val Value) {
	c.registerKlassInfo(name, val, staticMember)
}
func (c *ClassBluePrint) RegisterConstMember(name string, val Value) {
	c.registerKlassInfo(name, val, constMember)
}
func (c *ClassBluePrint) registerKlassInfo(name string, val Value, _type KlassType) {
	if _type != constMember {
		c.storeInContainer(name, val, _type)
	}
	checkAndStoreFunc := func(handle func(f *Function)) {
		if function, b := ToFunction(val); b {
			handle(function)
		} else {
			log.Warnf("register klass method fail: not function")
		}
	}
	switch _type {
	case constMember:
		c.ConstValue[name] = val
	case magicMethod:
		checkAndStoreFunc(func(f *Function) {
			if slices.Contains(c._container.GetProgram().magicMethodName, name) {
				c.magicMethod[name] = f
			} else {
				c.Method[name] = f
			}
		})
	case staticMember:
		c.StaticMember[name] = val
	case staticMethod:
		checkAndStoreFunc(func(f *Function) {
			c.StaticMethod[name] = f
		})
	case method_:
		checkAndStoreFunc(func(f *Function) {
			c.Method[name] = f
		})
	case member:
		val.GetProgram().SetInstructionWithName(name, val)
		c.NormalMember[name] = &BluePrintMember{
			Value: val,
			Type:  val.GetType(),
		}
	}
}

// storeInContainer store static in global container
func (c *ClassBluePrint) storeInContainer(name string, val Value, _type KlassType) {
	createVariable := func(builder *FunctionBuilder, variable *Variable) {
		builder.AssignVariable(variable, val)
	}
	//todo: extends seem error
	switch _type {
	case staticMethod, staticMember:
		builder := c._staticContainer.GetFunc().builder.GetMainBuilder()
		createVariable(builder, builder.CreateMemberCallVariable(c._staticContainer, builder.EmitConstInst(name)))
	default:
		builder := c._container.GetFunc().builder
		createVariable(builder, builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(name)))
	}
}
func (c *ClassBluePrint) StaticGeneratePhi() {
	lo.ForEach(lo.Entries(c._staticContainer.GetAllMember()), func(item lo.Entry[Value, Value], index int) {
		if _, ok := c.StaticMember[item.Key.String()]; ok {
			c._shadowMember[item.Key.String()] = append(c._shadowMember[item.Key.String()], item.Value)
		} else {
			//todo: add all member
			log.Warnf("not found this klsMember in kls")
		}
	})
	lo.ForEach(lo.Entries(c._shadowMember), func(item lo.Entry[string, Values], index int) {
		builder := c._staticContainer.GetFunc().builder.GetMainBuilder()
		phi := builder.EmitPhi(item.Key, []Value{})
		for _, value := range item.Value {
			ReplaceAllValue(value, phi)
		}
		phi.Edge = item.Value
		c.StaticMember[item.Key] = phi

	})
}

// ExecMagicMethod hook call
func (c *ClassBluePrint) ExecMagicMethod(handle func(method Value), name string) {
	if magicMethod := c.GetMagicMethod(name); !utils.IsNil(magicMethod) {
		handle(magicMethod)
	} else if method_ := c.GetMethod_(name); !utils.IsNil(method_) {
		log.Infof("not found this method in normal function")
		handle(method_)
	} else {
		log.Warn("not found this normal function")
	}
}
func (c *ClassBluePrint) SyntaxMethods() {
	lo.ForEach(c.ParentClass, func(item *ClassBluePrint, index int) {
		item.SyntaxMethods()
	})
	syntaxHandler := func(functions ...map[string]*Function) {
		lo.ForEach(functions, func(item map[string]*Function, index int) {
			for _, function := range item {
				function.Build()
				function.FixSpinUdChain()
			}
		})
	}
	checkAndGetMaps := func(vals ...Value) map[string]*Function {
		var results = make(map[string]*Function)
		lo.ForEach(vals, func(item Value, index int) {
			if funcs, b := ToFunction(c.Constructor); b {
				results[uuid.NewString()] = funcs
			}
		})
		return results
	}
	syntaxHandler(c.StaticMethod, c.Method, checkAndGetMaps(c.Constructor, c.Destructor))
}

func (b *ClassBluePrint) InitializeWithContainer(con *Make) error {
	if b._container != nil {
		return utils.Errorf("the container is already initialized id:(%v)", b._container.GetId())
	}
	b._container = con
	return nil
}
func (b *ClassBluePrint) InitStaticContainer(container *Make) {
	b._staticContainer = container
}
func (b *ClassBluePrint) GetClassContainer() Value {
	return b._container
}
func (c *ClassBluePrint) GetStaticContainer() Value {
	return c._staticContainer
}

func NewClassBluePrint() *ClassBluePrint {
	class := &ClassBluePrint{
		NormalMember: make(map[string]*BluePrintMember),
		StaticMember: make(map[string]Value),

		Method:           make(map[string]*Function),
		magicMethodCheck: make(map[string]func(val Value) bool),
		StaticMethod:     make(map[string]*Function),
		fullTypeName:     make([]string, 0),
		ConstValue:       make(map[string]Value),
		_shadowMember:    make(map[string]Values),
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
	if f, ok := c.Method[key]; ok {
		Point(fun, f)
	}
	c.Method[key] = fun
}

func (c *ClassBluePrint) GetMethod() map[string]*Function {
	return c.Method
}

func (c *ClassBluePrint) SetMethodGetter(f func() map[string]*Function) {
}

func (c *ClassBluePrint) AddFullTypeName(name string) {
	if c == nil {
		return
	}

	c.fullTypeName = append(c.fullTypeName, name)
}

func (c *ClassBluePrint) GetFullTypeNames() []string {
	if c == nil {
		return nil
	}
	return c.fullTypeName
}

func (c *ClassBluePrint) SetFullTypeNames(names []string) {
	if c == nil {
		return
	}
	c.fullTypeName = names
}
