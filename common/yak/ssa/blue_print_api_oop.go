package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (pkg *Program) GetBluePrint(name string) *BluePrint {
	if pkg == nil {
		return nil
	}
	if c, ok := pkg.ClassBluePrint[name]; ok {
		return c
	}
	// log.Errorf("GetClassBluePrint: not this class: %s", name)
	return nil
}

func (b *FunctionBuilder) GetBluePrint(name string) *BluePrint {
	p := b.prog
	return p.GetBluePrint(name)
}

func (b *FunctionBuilder) SetClassBluePrint(name string, class *BluePrint) {
	p := b.prog
	if _, ok := p.ClassBluePrint[name]; ok {
		log.Errorf("SetClassBluePrint: this class redeclare")
	}
	p.ClassBluePrint[name] = class
}

// CreateClassBluePrint will create object template (maybe class)
// in dynamic and classless language, we can create object without class
// but because of the 'this/super', we will still keep the concept 'Class'
// for ref the method/function, the blueprint is a container too,
// saving the static variables and util methods.

func (b *FunctionBuilder) CreateBluePrint(name string, tokenizer ...CanStartStopToken) *BluePrint {
	p := b.prog
	cls := b.createBlurPrintEx(name, tokenizer...)
	p.ClassBluePrint[name] = cls
	return cls
}
func (b *FunctionBuilder) createBlurPrintEx(name string, tokenizer ...CanStartStopToken) *BluePrint {
	c := NewClassBluePrint(name)
	c.GeneralUndefined = func(s string) *Undefined {
		return b.EmitUndefined(s)
	}
	klassVar := b.CreateVariable(name, tokenizer...)
	klassContainer := b.EmitEmptyContainer()
	b.AssignVariable(klassVar, klassContainer)
	_ = c.InitializeWithContainer(klassContainer)
	return c
}

// ReadSelfMember  用于读取当前类成员，包括静态成员和普通成员和方法。
// 其中使用MarkedThisClassBlueprint标识当前在哪个类中。
func (b *FunctionBuilder) ReadSelfMember(name string) Value {
	var value Value
	defer func() {
		if !utils.IsNil(value) {
			b.AssignVariable(b.CreateVariable(name), value)
		}
	}()
	if class := b.MarkedThisClassBlueprint; class != nil {
		variable := b.GetStaticMember(class, name)
		if _value := b.PeekValueByVariable(variable); _value != nil {
			return _value
		}
		if val := class.GetStaticMember(name); !utils.IsNil(val) {
			return val
		}
		if normalMember := class.GetNormalMember(name); !utils.IsNil(normalMember) {
			return normalMember
		}
		if method_ := class.GetNormalMethod(name); !utils.IsNil(method_) {
			return method_
		}
	}
	return nil
}
