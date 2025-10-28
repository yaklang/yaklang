//go:build !no_language
// +build !no_language

package php2ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

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
		variable := y.CreateVariable(name)
		variable.Assign(value)
		y.GetProgram().SetExportValue(name, value)
	}
	return nil
}
func (y *builder) VisitNamespaceOnlyUse(raw phpparser.INamespaceDeclarationContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	i, _ := raw.(*phpparser.NamespaceDeclarationContext)
	if i == nil {
		return
	}
	usedeclHanlder := func() {
		for _, statementContext := range i.AllNamespaceStatement() {
			stmt, ok := statementContext.(*phpparser.NamespaceStatementContext)
			if ok {
				y.VisitUseDeclaration(stmt.UseDeclaration())
			}
		}
	}
	prog := y.GetProgram().GetApplication() //拿到主app
	nameSpacePath := y.VisitNamespacePath(i.NamespacePath())
	namespaceName := strings.Join(nameSpacePath, ".")
	if len(nameSpacePath) == 0 {
		usedeclHanlder()
		return
	}
	library, b := prog.GetLibrary(namespaceName)
	if b {
		functionBuilder := library.GetAndCreateFunctionBuilder(namespaceName, string(ssa.InitFunctionName))
		currentBuilder := y.FunctionBuilder
		y.FunctionBuilder = functionBuilder
		usedeclHanlder()
		defer func() {
			y.FunctionBuilder = currentBuilder
		}()
	}
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
		library, _ := prog.GetLibrary(namespaceName)
		if library == nil {
			library = prog.NewLibrary(namespaceName, []string{prog.Loader.GetBasePath()})
		}
		//if custom syntax, only syntax it
		library.PushEditor(prog.GetCurrentEditor())
		functionBuilder := library.GetAndCreateFunctionBuilder(namespaceName, string(ssa.InitFunctionName))
		functionBuilder.SetEditor(y.FunctionBuilder.GetEditor())
		functionBuilder.SetBuildSupport(y.FunctionBuilder)
		currentBuilder := y.FunctionBuilder
		y.FunctionBuilder = functionBuilder
		return library, func() {
			library.VisitAst(raw)
			y.FunctionBuilder = currentBuilder
		}
	}

	switch {
	case hasName && y.PreHandler():
		// has name, and in pre-handler,  build this namespace and set lazyBuild in function
		y.callback(namespaceName, y.FunctionBuilder.GetEditor().GetFilename())
		_, f := switchToNamespace()
		defer f()
		program := y.GetProgram()
		program.PkgName = namespaceName
		declareStatement()
	case hasName && !y.PreHandler():
		// this statenment should effect on outter
		namespace, f := switchToNamespace()
		f()
		currentProg := y.GetProgram()
		y.SetProgram(namespace)
		normalStatement()
		y.SetProgram(currentProg)
	case !hasName && !y.PreHandler():
		prog.PkgName = namespaceName
		// build this un-name namespace
		declareStatement()
		normalStatement()
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
	checkNamespace := func(name ...string) *ssa.Program {
		namespaceName := strings.Join(name, ".")
		namespace, exit := prog.GetLibrary(namespaceName)
		if namespace == nil || !exit {
			return nil
		}
		return namespace
	}
	getOrCreateNamespace := func(name ...string) *ssa.Program {
		program := checkNamespace(name...)
		if !utils.IsNil(program) {
			return program
		}
		namespaceName := strings.Join(name, ".")
		library, err := prog.GetOrCreateLibrary(namespaceName)
		if err != nil {
			return nil
		} else {
			return library
		}
	}
	for _, listContext := range list.AllNamespaceNameList() {
		path, aliasMap := y.VisitNamespaceNameList(listContext)
		namespace := getOrCreateNamespace(path...)
		if namespace == nil {
			log.Warnf("namespace %s not found", path)
		}

		for realName, currentName := range aliasMap {
			switch opmode {
			case "const", "function":
				if namespace != nil {
					if err := prog.ImportValueFromLib(namespace, realName); err != nil {
						log.Errorf("get namespace value fail: %s", err)
					}
				}
			default:
				//有两种情况，cls、整个命名空间和下面的
				if cls := y.GetProgram().GetBluePrint(currentName); !utils.IsNil(cls) {
					log.Warnf("current builder has classblue: %s", cls)
					continue
				}
				if namespace != nil {
					if err := prog.ImportTypeFromLib(namespace, realName, listContext); err != nil {
						log.Errorf("get namespace type fail: %s", err)
					}
				}

				if namespace := checkNamespace(append(path, realName)...); namespace != nil {
					if err := prog.ImportAll(namespace); err != nil {
						log.Errorf("get namespace all fail: %s", err)
						return nil
					}

					//todo:
					for _, value := range namespace.ExportValue {
						if function, b := ssa.ToFunction(value); b {
							name := fmt.Sprintf("%s\\%s", currentName, function.GetName())
							prog.Funcs.Set(name, function)
						}
					}
					for _, t := range namespace.ExportType {
						if bluePrint, ok := t.(*ssa.Blueprint); ok {
							name := fmt.Sprintf("%s\\%s", currentName, bluePrint.Name)
							prog.Blueprint.Set(name, bluePrint)
						}
					}

					return nil
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
