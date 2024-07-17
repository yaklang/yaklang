package java2ssa

import (
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitInterfaceDeclaration(raw javaparser.IInterfaceDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.InterfaceDeclarationContext)
	if i == nil {
		return nil
	}

	ifaceName := i.Identifier().GetText()
	interfaceClass := y.CreateClassBluePrint(ifaceName)
	y.VisitInterfaceBody(i.InterfaceBody().(*javaparser.InterfaceBodyContext), interfaceClass)

	return nil
}

func (y *builder) VisitInterfaceBody(c *javaparser.InterfaceBodyContext, this *ssa.ClassBluePrint) interface{} {
	if y == nil || c == nil {
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

			fakeFunc := y.NewFunc(member.Identifier().GetText())
			fakeFunc.SetMethodName(member.Identifier().GetText())
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
				fakeFunc.Return = append(fakeFunc.Return, fakeRet)
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
			raw, ok := ret.(*javaparser.GenericInterfaceMethodDeclarationContext)
			if !ok {
				continue
			}

			// String url = app.GetProperties("jdbc.url");
			// String url = "jdbc... from file";
			//
			// JDBCConnectionBuilder = JDBC....getConnection(url);
			//
			// app.GetProper*(*?{opcode: const} as $literalConfig)
			// ${/(\.xml)|(.proper.*?)/}.regexp(result: $literalConfig<buildExpr>)
			// $result?{!(any: "127.0.0.1",localhost,*.internel)} as $dangerous
			// check $dangerout
			//
			// class{
			//   @Properties("aliyunoss.ak")
			//   String accessKey = null;
			// }
			//
			//

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
			// TODO: GenericInterfaceMethodDeclaration
			// handler(memberName, val)
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

func (y *builder) VisitConstantDeclarator(c *javaparser.ConstantDeclaratorContext) (string, []ssa.Value) {
	if y == nil || c == nil {
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
				m = y.EmitMakeWithoutType(y.EmitConstInst(0), y.EmitConstInst(0))
			} else {
				m = y.EmitMakeSlice(m, y.EmitConstInst(0), y.EmitConstInst(0), y.EmitConstInst(0))
			}
		}
		if m != nil {
			results = append(results, m)
		}
	}
	return name, results
}
