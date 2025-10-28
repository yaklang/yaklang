//go:build !no_language
// +build !no_language

package java2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *singleFileBuilder) VisitInterfaceDeclaration(raw javaparser.IInterfaceDeclarationContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return y.EmitEmptyContainer()
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.InterfaceDeclarationContext)
	if i == nil {
		return y.EmitEmptyContainer()
	}

	name := i.Identifier().GetText()
	blueprint := y.CreateBlueprint(name, i.Identifier())
	blueprint.SetKind(ssa.BlueprintInterface)
	y.GetProgram().SetExportType(name, blueprint)
	var extendNames []string
	tokenMap := make(map[string]ssa.CanStartStopToken)
	if i.EXTENDS() != nil {
		for _, extend := range i.AllTypeList() {
			extendNames = append(extendNames, extend.GetText())
			tokenMap[extend.GetText()] = extend
		}
	}

	store := y.StoreFunctionBuilder()
	blueprint.AddLazyBuilder(func() {
		switchHandler := y.SwitchFunctionBuilder(store)
		defer switchHandler()

		for _, extendName := range extendNames {
			bp := y.GetBluePrint(extendName)
			if utils.IsNil(bp) {
				bp = y.CreateBlueprint(extendName, tokenMap[extendName])
				y.AddFullTypeNameForAllImport(extendName, bp)
			}
			bp.SetKind(ssa.BlueprintInterface)
			blueprint.AddParentBlueprint(bp)
		}
	})
	y.MarkedThisClassBlueprint = blueprint
	defer func() {
		y.MarkedThisClassBlueprint = nil
	}()
	y.VisitInterfaceBody(i.InterfaceBody().(*javaparser.InterfaceBodyContext), blueprint)
	container := blueprint.Container()
	return container
}

func (y *singleFileBuilder) VisitInterfaceBody(c *javaparser.InterfaceBodyContext, this *ssa.Blueprint) interface{} {
	if y == nil || c == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(c)
	defer recoverRange()

	for _, decIf := range c.AllInterfaceBodyDeclaration() {
		memberRaw, ok := decIf.(*javaparser.InterfaceBodyDeclarationContext)
		if !ok {
			continue
		}

		cbs, defs, isStatic := y.VisitModifiers(memberRaw.Modifiers())
		_ = isStatic

		recv, ok := memberRaw.InterfaceMemberDeclaration().(*javaparser.InterfaceMemberDeclarationContext)
		if !ok {
			continue
		}
		if ret := recv.RecordDeclaration(); ret != nil {
			recoverRange := y.SetRange(ret)
			defer recoverRange()
			record := ret.(*javaparser.RecordDeclarationContext)
			if name, vals := y.VisitRecordDeclaration(record); name != "" {
				for _, val := range vals {
					if val != nil {
						for _, cb := range cbs {
							cb(val)
						}
						for _, def := range defs {
							def(val)
						}
					}
					// TODO: record in interface
					// handler(name, val)
				}
			}
		} else if ret := recv.ConstDeclaration(); ret != nil {
			recoverRange := y.SetRange(ret)
			defer recoverRange()
			member := ret.(*javaparser.ConstDeclarationContext)
			valType := y.VisitTypeType(member.TypeType())
			if valType != nil {

			}
			for _, constDec := range member.AllConstantDeclarator() {
				name, vals := y.VisitConstantDeclarator(constDec.(*javaparser.ConstantDeclaratorContext))
				if name == "" {
					continue
				}
				for _, val := range vals {
					val.SetType(valType)
					if val != nil {
						for _, cb := range cbs {
							cb(val)
						}
						for _, def := range defs {
							def(val)
						}
					}
					// TODO: const  in interface
					// handler(name, val)
				}
			}
		} else if ret := recv.InterfaceMethodDeclaration(); ret != nil {
			recoverRange := y.SetRange(ret)
			defer recoverRange()
			raw := ret.(*javaparser.InterfaceMethodDeclarationContext)
			if raw == nil {
				continue
			}
			var insCallbacks, defCallbacks []func(ssa.Value)
			for _, modRaw := range raw.AllInterfaceMethodModifier() {
				ins, ok := modRaw.(*javaparser.InterfaceMethodModifierContext)
				if !ok {
					continue
				}
				if ins.Annotation() != nil {
					insCallback, defCallback := y.VisitAnnotation(ins.Annotation())
					if insCallback != nil {
						insCallbacks = append(insCallbacks, insCallback)
					}
					if defCallback != nil {
						defCallbacks = append(defCallbacks, defCallback)
					}
				}
			}

			member, ok := raw.InterfaceCommonBodyDeclaration().(*javaparser.InterfaceCommonBodyDeclarationContext)
			if !ok {
				continue
			}
			{
				recoverRange := y.SetRange(member)
				defer recoverRange()
			}

			fakeFunc := y.NewFunc(member.Identifier().GetText())
			fakeFunc.SetMethodName(member.Identifier().GetText())
			fakeFunc.AddThrow(y.VisitThrowsClause(member)...)
			y.FunctionBuilder = y.PushFunction(fakeFunc)
			thisPara := y.NewParam("this", raw)
			thisPara.SetType(this)
			y.VisitFormalParameters(member.FormalParameters())
			y.VisitMethodBody(member.MethodBody())
			if len(fakeFunc.Return) <= 0 {
				retVal := y.EmitUndefined("")
				if t := y.VisitTypeTypeOrVoid(member.TypeTypeOrVoid()); t != nil {
					retVal.SetType(t)
				}
				fakeRet := y.EmitReturn([]ssa.Value{retVal})
				fakeFunc.Return = append(fakeFunc.Return, fakeRet.GetId())
			}
			y.Finish()
			y.FunctionBuilder = y.PopFunction()

			for _, anno := range member.AllAnnotation() {
				ins, def := y.VisitAnnotation(anno)
				if ins != nil {
					insCallbacks = append(insCallbacks, ins)
				}
				if def != nil {
					defCallbacks = append(defCallbacks, def)
				}
			}

			memberName := member.Identifier().GetText()
			val := fakeFunc
			for _, ins := range insCallbacks {
				ins(val)
			}
			for _, def := range defCallbacks {
				def(val)
			}
			for _, cb := range cbs {
				cb(val)
			}
			for _, def := range defs {
				def(val)
			}
			this.AddMethod(memberName, val)
			// handler(memberName, val)
		} else if ret := recv.GenericInterfaceMethodDeclaration(); ret != nil {
			recoverRange := y.SetRange(ret)
			defer recoverRange()
			raw, ok := ret.(*javaparser.GenericInterfaceMethodDeclarationContext)
			if !ok {
				continue
			}
			var insCallbacks, defCallbacks []func(ssa.Value)
			for _, modRaw := range raw.AllInterfaceMethodModifier() {
				ins, ok := modRaw.(*javaparser.InterfaceMethodModifierContext)
				if !ok {
					continue
				}
				if ins.Annotation() != nil {
					insCallback, defCallback := y.VisitAnnotation(ins.Annotation())
					if insCallback != nil {
						insCallbacks = append(insCallbacks, insCallback)
					}
					if defCallback != nil {
						defCallbacks = append(defCallbacks, defCallback)
					}
				}
			}

			member, ok := raw.InterfaceCommonBodyDeclaration().(*javaparser.InterfaceCommonBodyDeclarationContext)
			if !ok {
				continue
			}
			{
				recoverRange := y.SetRange(member)
				defer recoverRange()
			}
			for _, anno := range member.AllAnnotation() {
				ins, def := y.VisitAnnotation(anno)
				if ins != nil {
					insCallbacks = append(insCallbacks, ins)
				}
				if def != nil {
					defCallbacks = append(defCallbacks, def)
				}
			}

			memberName := member.Identifier().GetText()
			val := y.EmitUndefined(memberName)
			for _, ins := range insCallbacks {
				ins(val)
			}
			for _, def := range defCallbacks {
				def(val)
			}
		} else if ret := recv.InterfaceDeclaration(); ret != nil {
			y.VisitInterfaceDeclaration(ret.(*javaparser.InterfaceDeclarationContext))
		} else if ret := recv.AnnotationTypeDeclaration(); ret != nil {

		} else if ret := recv.ClassDeclaration(); ret != nil {
			y.VisitClassDeclaration(ret.(*javaparser.ClassDeclarationContext), this)
		} else if ret := recv.EnumDeclaration(); ret != nil {
			y.VisitEnumDeclaration(ret, this)
		}
	}

	return nil
}

func (y *singleFileBuilder) VisitConstantDeclarator(c *javaparser.ConstantDeclaratorContext) (string, []ssa.Value) {
	if y == nil || c == nil || y.IsStop() {
		return "", nil
	}
	recoverRange := y.SetRange(c)
	defer recoverRange()

	name := c.Identifier().GetText()
	initVal := y.VisitVariableInitializer(c.VariableInitializer())
	var results []ssa.Value
	if initVal != nil {
		results = append(results, initVal)
	}
	if dim := len(c.AllLBRACK()); dim > 0 {
		// array
		var m ssa.Value
		if initVal != nil {
			m = initVal
		}
		for i := 0; i < dim; i++ {
			if m == nil {
				m = y.EmitMakeWithoutType(y.EmitConstInstPlaceholder(0), y.EmitConstInstPlaceholder(0))
			} else {
				m = y.EmitMakeSlice(m, y.EmitConstInstPlaceholder(0), y.EmitConstInstPlaceholder(0), y.EmitConstInstPlaceholder(0))
			}
		}
		if m != nil {
			results = append(results, m)
		}
	}
	return name, results
}
