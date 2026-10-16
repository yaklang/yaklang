package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"

	"golang.org/x/exp/slices"
)

type BlueprintMagicMethodKind string

const (
	Constructor BlueprintMagicMethodKind = "constructor"
	Destructor                           = "destructor"
)

// magic
func (c *Blueprint) IsMagicMethodName(name BlueprintMagicMethodKind) bool {
	if c == nil || utils.IsNil(c._container) {
		return false
	}
	program := c._container.GetProgram()
	return program != nil && slices.Contains(program.magicMethodName, string(name))
}

func (c *Blueprint) RegisterMagicMethod(name BlueprintMagicMethodKind, val Value) {
	c.registerMagicMethod(name, val)
}

// registerMagicMethod publishes the slot before any builder call. storeField
// and Point run after SafeMap's lock is released: member lowering re-enters
// GetMagicMethod on this goroutine, and a package-level mutex there deadlocks.
func (c *Blueprint) registerMagicMethod(name BlueprintMagicMethodKind, val Value) {
	if c == nil || utils.IsNil(val) {
		return
	}
	if !c.IsMagicMethodName(name) {
		log.Debugf("register magic method fail: not magic method: %q", name)
		//return
	}
	if c.MagicMethod == nil {
		c.MagicMethod = utils.NewSafeMapWithKey[BlueprintMagicMethodKind, Value]()
	}
	c.MagicMethod.Update(name, func(old Value, loaded bool) (Value, bool) {
		switch name {
		case Constructor:
			c.Constructor = val
		case Destructor:
			c.Destructor = val
		}
		if !loaded || utils.IsNil(old) {
			return val, true
		}
		if val.GetId() != old.GetId() {
			alreadyLinked := false
			if reference := val.GetReference(); !utils.IsNil(reference) && reference.GetId() == old.GetId() {
				for _, pointer := range old.GetPointer() {
					if !utils.IsNil(pointer) && pointer.GetId() == val.GetId() {
						alreadyLinked = true
						break
					}
				}
			}
			if !alreadyLinked {
				Point(val, old)
			}
		}
		return old, false
	})
	switch name {
	case Constructor:
		c.storeField(c.Name, val, BluePrintMagicMethod)
		val.SetVerboseName(c.Name)
		return
	case Destructor:
	}
	c.storeField(val.GetName(), val, BluePrintMagicMethod)
}

func (c *Blueprint) readMagicMethod(name BlueprintMagicMethodKind) Value {
	if c == nil || c.MagicMethod == nil {
		return nil
	}
	var out Value
	c.MagicMethod.Update(name, func(old Value, _ bool) (Value, bool) {
		switch name {
		case Constructor:
			if !utils.IsNil(c.Constructor) {
				out = c.Constructor
				break
			}
		case Destructor:
			if !utils.IsNil(c.Destructor) {
				out = c.Destructor
				break
			}
		}
		if utils.IsNil(out) {
			out = old
		}
		return old, false
	})
	return out
}

func (c *Blueprint) GetMagicMethod(name BlueprintMagicMethodKind, fb *FunctionBuilder) Value {
	method := c.resolveMagicMethod(name, fb)
	if !utils.IsNil(method) {
		if function, ok := ToFunction(method); ok {
			function.buildOnLookup()
		}
	}
	return method
}

func (c *Blueprint) resolveMagicMethod(name BlueprintMagicMethodKind, fb *FunctionBuilder) Value {
	var _method Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		value := bluePrint.readMagicMethod(name)
		if utils.IsNil(value) {
			return false
		}
		// External and detached blueprints can have a constructor
		// without a container function. Only an actual virtual owner
		// makes that constructor ineligible for lookup.
		if name == Constructor {
			if container := bluePrint._container; !utils.IsNil(container) {
				if owner := container.GetFunc(); owner != nil && owner.name == string(VirtualFunctionName) {
					return false
				}
			}
		}
		_method = value
		return true
	})
	if utils.IsNil(_method) {
		if c.Range != nil {
			backup := fb.CurrentRange
			fb.CurrentRange = c.Range
			defer func() {
				fb.CurrentRange = backup
			}()
		}

		switch name {
		case Constructor:
			_name := fmt.Sprintf("%s-constructor", c.Name)
			//constructor := c.GeneralUndefined(_name)
			constructor := fb.EmitUndefined(_name)
			_method = constructor
			functionType := NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true)
			functionType.SetFullTypeNames(c.GetFullTypeNames())
			_method.SetType(functionType)
			c.registerMagicMethod(Constructor, _method)
		case Destructor:
			_name := fmt.Sprintf("%s-destructor", c.Name)
			destructor := fb.EmitUndefined(_name)
			_method = destructor
			functionType := NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true)
			functionType.SetFullTypeNames(c.GetFullTypeNames())
			_method.SetType(functionType)
			c.registerMagicMethod(Destructor, _method)
		default:
			return nil
		}
	}
	return _method
}

// normal method
func (c *Blueprint) RegisterNormalMethod(name string, val *Function, store ...bool) {
	if len(store) == 0 || store[0] == true {
		c.storeField(name, val, BluePrintNormalMethod)
	}
	if f, ok := ToFunction(val); ok {
		f.SetMethod(true, c)
	}
	if method := c.NormalMethod[name]; !utils.IsNil(method) {
		Point(method, val)
	} else {
		c.NormalMethod[name] = val
	}
}

// RegisterNormalMethodExact installs one callable in the blueprint slot without
// adding a pointer/reference edge to the previous slot value. Frontends with
// their own overload registry use this to keep overload edges distinct from
// virtual-dispatch edges.
func (c *Blueprint) RegisterNormalMethodExact(name string, val *Function, store ...bool) {
	if len(store) == 0 || store[0] {
		c.storeField(name, val, BluePrintNormalMethod)
	}
	if f, ok := ToFunction(val); ok {
		f.SetMethod(true, c)
	}
	c.NormalMethod[name] = val
}

func (c *Blueprint) GetNormalMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if function, ok := bluePrint.NormalMethod[key]; ok {
			f = function
			function.buildOnLookup()
			return true
		}
		return false
	})
	return f
}

// static method
func (c *Blueprint) RegisterStaticMethod(name string, val *Function, store ...bool) {
	if method := c.StaticMethod[name]; !utils.IsNil(method) {
		Point(method, val)
	} else {
		if len(store) == 0 || store[0] {
			c.storeField(name, val, BluePrintStaticMember)
		}
		c.StaticMethod[name] = val
	}
}

// RegisterStaticMethodExact is the static counterpart of
// RegisterNormalMethodExact.
func (c *Blueprint) RegisterStaticMethodExact(name string, val *Function, store ...bool) {
	if len(store) == 0 || store[0] {
		c.storeField(name, val, BluePrintStaticMember)
	}
	c.StaticMethod[name] = val
}

func (c *Blueprint) GetStaticMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if function, ok := bluePrint.StaticMethod[key]; ok {
			f = function
			function.buildOnLookup()
			return true
		}
		return false
	})
	return f
}
