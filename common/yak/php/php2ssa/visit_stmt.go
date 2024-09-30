package php2ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
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
	//custom syntax
	beforfunc := func() {
		for _, statementContext := range i.AllNamespaceStatement() {
			y.BeforeVisitNamespaceStatement(statementContext)
		}
	}
	afterFunc := func() {
		for _, statementContext := range i.AllNamespaceStatement() {
			y.VisitNamesPaceStatement(statementContext)
		}
	}
	//compose child app
	if i.NamespacePath() != nil && y.PreHandler() {
		beforfunc()
		//namespace
		pkgpath := y.VisitNamespacePath(i.NamespacePath())
		pkgname := strings.Join(pkgpath, ".")
		y.callback(pkgname, y.FunctionBuilder.GetEditor().GetFilename())
		prog := y.GetProgram().GetApplication() //拿到主app
		library, _ := prog.GetLibrary(pkgname)
		if library == nil {
			library = prog.NewLibrary(pkgname, []string{prog.Loader.GetBasePath()})
		}
		//if custom syntax, only syntax it
		library.PushEditor(prog.GetCurrentEditor())
		functionBuilder := library.GetAndCreateFunctionBuilder(pkgname, "init")
		functionBuilder.SetEditor(y.FunctionBuilder.GetEditor())
		if functionBuilder != nil {
			functionBuilder.SetBuildSupport(y.FunctionBuilder)
			currentBuilder := y.FunctionBuilder
			y.FunctionBuilder = functionBuilder
			defer func() {
				y.FunctionBuilder = currentBuilder
			}()
			afterFunc()
		}
	} else {
		if y.PreHandler() {
			return nil
		}
		beforfunc()
		afterFunc()
	}
	return nil
}

func (y *builder) VisitNamesPaceStatement(raw phpparser.INamespaceStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceStatementContext)
	if i == nil {
		return nil
	}
	y.VisitUseDeclaration(i.UseDeclaration())
	//y.VisitStatement(i.Statement())
	y.VisitFunctionDeclaration(i.FunctionDeclaration())
	y.VisitClassDeclaration(i.ClassDeclaration())
	y.VisitGlobalConstantDeclaration(i.GlobalConstantDeclaration())
	return nil
}
func (y *builder) BeforeVisitNamespaceStatement(raw phpparser.INamespaceStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceStatementContext)
	if i == nil {
		return nil
	}
	y.VisitUseDeclaration(i.UseDeclaration())
	y.VisitStatement(i.Statement())
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
	prog := y.GetProgram().GetApplication()
	y.VisitUseDeclarationContentList(i.UseDeclarationContentList(), func(path []string, aliasMap map[string]string) {
		defer func() {
			if msg := recover(); msg != nil {
				log.Errorf("visit use decl fail: %s", msg)
			}
		}()
		for old, alias := range aliasMap {
			switch opmode {
			case "const", "function":
				//todo const
				if function := y.GetProgram().GetFunction(alias); !utils.IsNil(function) {
					log.Warnf("current builder has function: %s", function.GetName())
					continue
				}
				if library, _ := prog.GetLibrary(strings.Join(path, ".")); !utils.IsNil(library) {
					if function := library.GetFunction(old); function == nil {
						log.Errorf("get lib function fail,name: %s,namespace path: %s", old, strings.Join(path, "."))
						continue
					} else {
						y.AssignVariable(y.CreateVariable(alias), function)
						y.GetProgram().Funcs[alias] = function
					}
				}
				//有两种情况，class或者整个命名空间
			default:
				if cls := y.GetProgram().GetClassBluePrint(alias); !utils.IsNil(cls) {
					log.Warnf("current builder has classblue: %s", cls)
					continue
				}
				if library, _ := prog.GetLibrary(strings.Join(path, ".")); library != nil {
					//一个类的情况
					if bluePrint := library.GetClassBluePrint(old); !utils.IsNil(bluePrint) {
						y.SetClassBluePrint(alias, bluePrint)
					} else {
						log.Warnf("lib get class: %s fail", old)
					}
				}
				if library, _ := prog.GetLibrary(strings.Join(append(path, old), ".")); library != nil {
					//todo: 可能会和常量重名
					for _, bluePrint := range library.ClassBluePrint {
						name := fmt.Sprintf("%s\\%s", old, bluePrint.Name)
						y.SetClassBluePrint(name, bluePrint)
					}
					for _, function := range library.Funcs {
						y.AssignVariable(y.CreateVariable(fmt.Sprintf("%s\\%s", old, function.GetName())), function)
					}
				}
			}
		}
	})
	return nil
}
func (y *builder) VisitUseDeclarationContentList(
	raw phpparser.IUseDeclarationContentListContext,
	callback func(path []string, aliasMap map[string]string)) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.UseDeclarationContentListContext)
	if i == nil {
		return nil
	}
	for _, listContext := range i.AllNamespaceNameList() {
		list, m := y.VisitNamespaceNameList(listContext)
		callback(list, m)
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
