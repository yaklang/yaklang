package php2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTopStatement(raw phpparser.ITopStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TopStatementContext)
	if i == nil {
		return nil
	}
	//custom file not syntax
	if y.PreHandler() && i.NamespaceDeclaration() == nil {
		return nil
	}
	y.VisitNamespaceDeclaration(i.NamespaceDeclaration())
	y.VisitGlobalConstantDeclaration(i.GlobalConstantDeclaration())
	y.VisitUseDeclaration(i.UseDeclaration())
	y.VisitFunctionDeclaration(i.FunctionDeclaration())
	y.VisitEnumDeclaration(i.EnumDeclaration())
	y.VisitClassDeclaration(i.ClassDeclaration())
	y.VisitStatement(i.Statement())
	return nil
}

func (y *builder) VisitEnumDeclaration(raw phpparser.IEnumDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.EnumDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitGlobalConstantDeclaration(raw phpparser.IGlobalConstantDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.GlobalConstantDeclarationContext)
	if i == nil {
		return nil
	}
	y.VisitAttributes(i.Attributes())
	for _, initializerContext := range i.AllIdentifierInitializer() {
		name, value := y.VisitIdentifierInitializer(initializerContext)
		y.AssignConst(name, value)
	}
	return nil
}

func (y *builder) VisitNamespaceDeclaration(raw phpparser.INamespaceDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceDeclarationContext)
	if i == nil {
		return nil
	}
	nameSpaceStmt := func(build func(*phpparser.NamespaceStatementContext)) {
		for _, stmt := range i.AllNamespaceStatement() {
			if i, ok := stmt.(*phpparser.NamespaceStatementContext); ok {
				build(i)
			}
		}
	}
	UseStatement := func() {
		nameSpaceStmt(func(nsc *phpparser.NamespaceStatementContext) {
			y.VisitUseDeclaration(nsc.UseDeclaration())
		})
	}
	normalStatement := func() {
		nameSpaceStmt(func(nsc *phpparser.NamespaceStatementContext) {
			y.VisitStatement(nsc.Statement())
		})
	}
	declareStatement := func() {
		nameSpaceStmt(func(i *phpparser.NamespaceStatementContext) {
			y.VisitFunctionDeclaration(i.FunctionDeclaration())
			y.VisitClassDeclaration(i.ClassDeclaration())
			y.VisitGlobalConstantDeclaration(i.GlobalConstantDeclaration())
		})
	}
	//compose child app
	hasName := i.NamespacePath() != nil

	prog := y.GetProgram().GetApplication() //拿到主app
	nameSpacePath := y.VisitNamespacePath(i.NamespacePath())
	namespaceName := strings.Join(nameSpacePath, ".")
	switchToNamespace := func() (*ssa.Program, func()) {
		library, ok := prog.GetLibrary(namespaceName)
		if library == nil || !ok {
			library = prog.NewLibrary(namespaceName, []string{prog.Loader.GetBasePath()})
		}
		//if custom syntax, only syntax it
		library.PushEditor(prog.GetCurrentEditor())
		functionBuilder := library.GetAndCreateFunctionBuilder(namespaceName, "init")
		functionBuilder.SetEditor(y.FunctionBuilder.GetEditor())
		functionBuilder.SetBuildSupport(y.FunctionBuilder)
		currentBuilder := y.FunctionBuilder
		y.FunctionBuilder = functionBuilder
		return library, func() {
			y.FunctionBuilder = currentBuilder
		}
	}

	switch {
	case hasName && y.PreHandler():
		// has name, and in pre-handler,  build this namespace and set lazyBuild in function
		y.callback(namespaceName, y.FunctionBuilder.GetEditor().GetFilename())
		_, f := switchToNamespace()
		defer f()
		declareStatement()
	case hasName && !y.PreHandler():
		// finish Function
		normalStatement()
		namespaceProg, f := switchToNamespace()
		defer f()
		UseStatement()
		for _, fun := range namespaceProg.Funcs {
			fun.Build()
		}
		for _, cls := range namespaceProg.ClassBluePrint {
			cls.Build()
		}
	case !hasName && !y.PreHandler():
		// build this un-name namespace
		UseStatement()
		normalStatement()
		declareStatement()
	}

	return nil
}

func (y *builder) VisitUseDeclaration(raw phpparser.IUseDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.UseDeclarationContext)
	if i == nil {
		return nil
	}
	var opmode string
	if i.GetOpmode() != nil {
		opmode = strings.TrimSpace(i.GetOpmode().GetText())
	} else {
		opmode = ""
	}
	prog := y.GetProgram()

	list, _ := i.UseDeclarationContentList().(*phpparser.UseDeclarationContentListContext)
	if list == nil {
		return nil
	}
	getNamespace := func(name ...string) *ssa.Program {
		namespaceName := strings.Join(name, ".")
		namespace, ok := prog.GetLibrary(namespaceName)
		if namespace == nil || !ok {
			return nil
		}
		return namespace
	}
	for _, listContext := range list.AllNamespaceNameList() {
		path, aliasMap := y.VisitNamespaceNameList(listContext)
		namespace := getNamespace(path...)
		if namespace == nil {
			log.Warnf("namespace %s not found", path)
		}

		for realName, currentName := range aliasMap {
			switch opmode {
			case "const", "function":
				//todo const

				if function := y.GetProgram().GetFunction(currentName); !utils.IsNil(function) {
					log.Warnf("current builder has function: %s", function.GetName())
					continue
				}
				if namespace != nil {
					if _, err := prog.ImportValue(namespace, realName); err != nil {
						log.Errorf("get namespace value fail: %s", err)
					}
				}
			default:
				//有两种情况，class或者整个命名空间
				if cls := y.GetProgram().GetClassBluePrint(currentName); !utils.IsNil(cls) {
					log.Warnf("current builder has classblue: %s", cls)
					continue
				}
				if namespace != nil {
					if _, err := prog.ImportType(namespace, realName); err != nil {
						log.Errorf("get namespace type fail: %s", err)
					}
				}

				if namespace := getNamespace(append(path, realName)...); namespace != nil {
					y.GetProgram().CurrentNameSpace = strings.Join(path, ".") + "."
					if err := prog.ImportAll(namespace); err != nil {
						log.Errorf("get namespace all fail: %s", err)
					}
					return namespace
				}
			}
		}
	}
	return nil
}

func (y *builder) VisitStatement(raw phpparser.IStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.StatementContext)
	if i == nil {
		return nil
	}

	if r := i.LabelStatement(); r != nil {
		y.VisitLabelStatement(r)
	} else if b := i.BlockStatement(); b != nil {
		y.VisitBlockStatement(b)
	} else if r := i.IfStatement(); r != nil {
		y.VisitIfStatement(r)
	} else if r := i.WhileStatement(); r != nil {
		y.VisitWhileStatement(r)
	} else if r := i.DoWhileStatement(); r != nil {
		y.VisitDoWhileStatement(r)
	} else if i.ForStatement() != nil {
		y.VisitForStatement(i.ForStatement())
	} else if r := i.SwitchStatement(); r != nil {
		y.VisitSwitchStatement(r)
	} else if r := i.BreakStatement(); r != nil {
		y.VisitBreakStatement(r)
	} else if r := i.ContinueStatement(); r != nil {
		y.VisitContinueStatement(r)
	} else if r := i.ReturnStatement(); r != nil {
		y.VisitReturnStatement(r)
	} else if i.YieldExpression() != nil {
		y.VisitYieldExpression(i.YieldExpression())
	} else if i.GlobalStatement() != nil {
		y.VisitGlobalStatement(i.GlobalStatement())
	} else if i.StaticVariableStatement() != nil {
		y.VisitStaticVariableStatement(i.StaticVariableStatement())
	} else if i.EchoStatement() != nil {
		y.VisitEchoStatement(i.EchoStatement())
	} else if i.ExpressionStatement() != nil {
		y.VisitExpressionStatement(i.ExpressionStatement())
	} else if i.UnsetStatement() != nil {
		y.VisitUnsetStatement(i.UnsetStatement())
	} else if i.ForeachStatement() != nil {
		y.VisitForeachStatement(i.ForeachStatement())
	} else if i.TryCatchFinally() != nil {
		y.VisitTryCatchFinally(i.TryCatchFinally())
	} else if i.ThrowStatement() != nil {
		y.VisitThrowStatement(i.ThrowStatement())
	} else if i.GotoStatement() != nil {
		y.VisitGotoStatement(i.GotoStatement())
	} else if i.DeclareStatement() != nil {
		y.VisitDeclareStatement(i.DeclareStatement())
	} else if i.ExpressionStatement() != nil {
		y.VisitExpressionStatement(i.ExpressionStatement())
	} else if i.EmptyStatement_() != nil {
		y.VisitEmptyStatement(i.EmptyStatement_())
	} else if i.InlineHtmlStatement() != nil {
		y.VisitInlineHtmlStatement(i.InlineHtmlStatement())
	} else {
		log.Infof("unknown statement: %v", i.GetText())
	}

	return nil
}

// VisitLabelStatement check id: as goto target
func (y *builder) VisitLabelStatement(raw phpparser.ILabelStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.LabelStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitBlockStatement(raw phpparser.IBlockStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.BlockStatementContext)
	if i == nil {
		return nil
	}

	y.VisitInnerStatementList(i.InnerStatementList())

	return nil
}

func (y *builder) VisitInnerStatementList(raw phpparser.IInnerStatementListContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.InnerStatementListContext)
	if i == nil {
		return nil
	}
	for _, is := range i.AllInnerStatement() {
		y.VisitInnerStatement(is)
	}

	return nil
}

func (y *builder) VisitInnerStatement(raw phpparser.IInnerStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.InnerStatementContext)
	if i == nil {
		return nil
	}

	if i.Statement() != nil {
		y.VisitStatement(i.Statement())
	} else if i.FunctionDeclaration() != nil {
		y.VisitFunctionDeclaration(i.FunctionDeclaration())
	} else if i.ClassDeclaration() != nil {
		y.VisitClassDeclaration(i.ClassDeclaration())
	} else {
		log.Infof("unknown inner statement: %v", i.GetText())
	}

	return nil
}
