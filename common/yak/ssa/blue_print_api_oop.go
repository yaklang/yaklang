package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// GetBluePrint will get the blueprint by name. if not found and virtualImport enable,
// it will try to create blueprint by name
func (pkg *Program) GetBluePrint(name string, token ...CanStartStopToken) *Blueprint {
	if pkg == nil {
		return nil
	}
	return pkg.GetClassBlueprintEx(name, "", token...)
}

func (b *FunctionBuilder) GetBluePrint(name string) *Blueprint {
	p := b.prog
	if bp := p.GetBluePrint(name); bp != nil {
		return bp
	}
	var blueprint *Blueprint
	b.includeStack.ForeachStack(func(program *Program) bool {
		if resultBlueprint, ok := program.Blueprint.Get(name); ok {
			blueprint = resultBlueprint
			return false
		}
		return true
	})
	return blueprint
}

func (b *FunctionBuilder) SetBlueprint(name string, class *Blueprint) {
	p := b.prog
	_, exit := p.Blueprint.Get(name)
	if exit {
		log.Errorf("SetBlueprint: this class redeclare")
	}
	p.Blueprint.Set(name, class)
}

// CreateBlueprintWithPkgName will create object template (maybe class)
// in dynamic and classless language, we can create object without class
// but because of the 'this/super', we will still keep the concept 'Class'
// for ref the method/function, the blueprint is a container too,
// saving the static variables and util methods.
func (b *FunctionBuilder) CreateBlueprintWithPkgName(name string, tokenizers ...CanStartStopToken) *Blueprint {
	var codeRange *memedit.Range
	if len(tokenizers) > 0 {
		tokenizer := tokenizers[0]
		codeRange = b.GetRangeByToken(tokenizer)
		recoverRange := b.SetRangePure(codeRange)
		defer recoverRange()
	}
	prog := b.prog
	blueprint := NewBlueprint(name)
	if prog.Blueprint == nil {
		prog.Blueprint = omap.NewEmptyOrderedMap[string, *Blueprint]()
	}

	blueprint.Range = codeRange

	b.SetBlueprint(name, blueprint)
	blueprintContainer := b.EmitEmptyContainer()
	blueprintContainer.SetName(name)
	blueprintContainer.SetVerboseName(name)
	blueprintContainer.SetType(blueprint)

	// search this blueprint-declare can use ${blueprint-name} or ${blueprint-name}_declare
	variableName := fmt.Sprintf("%s_declare", name)
	var1 := b.CreateVariable(variableName, tokenizers...)
	b.AssignVariable(var1, blueprintContainer)
	var2 := b.CreateVariable(name, tokenizers...)
	b.AssignVariable(var2, blueprintContainer)

	if err := blueprint.InitializeWithContainer(blueprintContainer); err != nil {
		log.Errorf("CreateBluePrintWithPkgName.InitializeWithContainer error: %s", err)
	}

	if b.IsVirtualImport() {
		//generate default fullTypeName
		packagename := b.GetProgram().PkgName
		if packagename == "" {
			packagename = "main"
		}
		defaultFullTypename := fmt.Sprintf("%s.%s", packagename, name)
		blueprint.AddFullTypeName(defaultFullTypename)
	}
	return blueprint
}

func (b *FunctionBuilder) CreateBlueprint(name string, tokenizer ...CanStartStopToken) *Blueprint {
	blueprint := b.CreateBlueprintWithPkgName(name, tokenizer...)
	blueprint.SetKind(BlueprintClass)
	return blueprint
}
func (b *FunctionBuilder) CreateInterface(name string, tokenizer ...CanStartStopToken) *Blueprint {
	blueprint := b.CreateBlueprint(name, tokenizer...)
	blueprint.SetKind(BlueprintInterface)
	return blueprint
}

func (b *FunctionBuilder) CreateBlueprintAndSetConstruct(typName string, libName ...string) *Blueprint {
	var name string
	if len(libName) > 0 {
		name = fmt.Sprintf("%s_%s", libName[0], typName)
	} else {
		name = typName
	}

	if bp := b.GetBluePrint(name); bp != nil {
		return bp
	}

	bp := b.CreateBlueprint(name)
	newFunction := b.NewFunc(typName)
	newFunction.SetMethodName(typName)
	newFunction.SetType(NewFunctionType(fmt.Sprintf("%s-__construct", typName), []Type{}, nil, true))
	bp.RegisterMagicMethod(Constructor, newFunction)
	return bp
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

func (b *FunctionBuilder) PushBlueprint(bp *Blueprint) {
	prog := b.GetProgram()
	if prog == nil {
		return
	}
	if prog.BlueprintStack == nil {
		prog.BlueprintStack = utils.NewStack[*Blueprint]()
	}
	prog.BlueprintStack.Push(bp)
}

func (b *FunctionBuilder) PeekInnerBlueprint() *Blueprint {
	prog := b.GetProgram()
	if prog == nil {
		return nil
	}
	if prog.BlueprintStack == nil {
		return nil
	}
	return prog.BlueprintStack.Peek()
}

func (b *FunctionBuilder) PopBlueprint() *Blueprint {
	prog := b.GetProgram()
	if prog == nil {
		return nil
	}
	if prog.BlueprintStack == nil {
		return nil
	}
	return prog.BlueprintStack.Pop()
}

func (b *FunctionBuilder) PeekNInnerBlueprint(n int) *Blueprint {
	prog := b.GetProgram()
	if prog == nil {
		return nil
	}
	if prog.BlueprintStack == nil {
		return nil
	}
	return prog.BlueprintStack.PeekN(n)
}

func (b *FunctionBuilder) FakeGetBlueprint(lib *Program, name string, token ...CanStartStopToken) *Blueprint {
	blueprintType := fakeGetType(lib, name, token...)
	blueprint, _ := ToClassBluePrintType(blueprintType)
	return blueprint
}
