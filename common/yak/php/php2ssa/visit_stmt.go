package php2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTopStatement(raw phpparser.ITopStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TopStatementContext)
	if i == nil {
		return nil
	}

	if ret := i.Statement(); ret != nil {
		y.VisitStatement(ret)
	} else if ret := i.UseDeclaration(); ret != nil {
		y.VisitUseDeclaration(ret)
	} else if ret := i.NamespaceDeclaration(); ret != nil {
		y.VisitNamespaceDeclaration(ret)
	} else if ret := i.FunctionDeclaration(); ret != nil {
		y.VisitFunctionDeclaration(ret)
	} else if ret := i.ClassDeclaration(); ret != nil {
		y.VisitClassDeclaration(ret)
	} else if ret := i.GlobalConstantDeclaration(); ret != nil {
		y.VisitGlobalConstantDeclaration(ret)
	} else if ret := i.EnumDeclaration(); ret != nil {
		y.VisitEnumDeclaration(ret)
	} else {
		log.Infof("unknown top statement: %v", i.GetText())
	}

	return nil
}

func (y *builder) VisitEnumDeclaration(raw phpparser.IEnumDeclarationContext) interface{} {
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceDeclarationContext)
	if i == nil {
		return nil
	}
	list := y.VisitNamespaceNameList(i.NamespaceNameList())
	pkgName := strings.Join(list, ".")
	if len(list) > 0 {
		for _, statementContext := range i.AllNamespaceStatement() {
			y.BeforeVisitNamespaceStatement(statementContext)
		}
	}
	if pkgName != "" {
		program := y.GetProgram()
		lib, _ := program.GetLibrary(pkgName)
		if lib == nil {
			lib = program.NewLibrary(pkgName, list)
		}
		lib.PushEditor(program.GetCurrentEditor())
		builder := lib.GetAndCreateFunctionBuilder(pkgName, "init")
		if builder != nil {
			builder.SetBuildSupport(y.FunctionBuilder)
			currentBuilder := y.FunctionBuilder
			y.FunctionBuilder = builder
			defer func() {
				y.FunctionBuilder = currentBuilder
			}()
		}
	}
	for _, statement := range i.AllNamespaceStatement() {
		y.VisitNamesPaceStatement(statement)
	}
	if len(list) == 0 {
		for _, statementContext := range i.AllNamespaceStatement() {
			y.BeforeVisitNamespaceStatement(statementContext)
		}
	}
	return nil
}

func (y *builder) VisitNamesPaceStatement(raw phpparser.INamespaceStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceStatementContext)
	if i == nil {
		return nil
	}
	//y.VisitStatement(i.Statement()) //statement有问题
	y.VisitUseDeclaration(i.UseDeclaration())
	y.VisitFunctionDeclaration(i.FunctionDeclaration())
	y.VisitClassDeclaration(i.ClassDeclaration())
	y.VisitGlobalConstantDeclaration(i.GlobalConstantDeclaration())
	return nil
}
func (y *builder) BeforeVisitNamespaceStatement(raw phpparser.INamespaceStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NamespaceStatementContext)
	if i == nil {
		return nil
	}
	y.VisitStatement(i.Statement())
	return nil
}
func (y *builder) VisitUseDeclaration(raw phpparser.IUseDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.UseDeclarationContext)
	if i == nil {
		return nil
	}
	y.VisitUseDeclarationContentList(i.UseDeclarationContentList())
	return nil
}
func (y *builder) VisitUseDeclarationContentList(raw phpparser.IUseDeclarationContentListContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.UseDeclarationContentListContext)
	if i == nil {
		return nil
	}
	for _, list := range i.AllNamespaceNameList() {
		pkgNames := y.VisitNamespaceNameList(list)
		var prog *ssa.Program
		lib := strings.Join(pkgNames, ".")
		if library, _ := y.GetProgram().GetLibrary(lib); library != nil {
			//import all
			for _, class := range prog.ClassBluePrint {
				y.SetClassBluePrint(class.Name, class)
			}
		} else {
			//import class
			lib := strings.Join(pkgNames[:len(pkgNames)-1], ".")
			if library, _ := y.GetProgram().GetLibrary(lib); library != nil {
				if bluePrint := library.GetClassBluePrint(pkgNames[len(pkgNames)-1]); bluePrint != nil {
					y.SetClassBluePrint(pkgNames[len(pkgNames)-1], bluePrint)
				}
			} else {
				log.Warnf("get namespace lib fail: %v", lib)
			}
		}
	}
	return nil
}

func (y *builder) VisitStatement(raw phpparser.IStatementContext) interface{} {
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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
