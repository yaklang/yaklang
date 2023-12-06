package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitTopStatement(raw phpparser.ITopStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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
		log.Infof("unknown top statement: %v", ret.GetText())
	}

	return nil
}

func (y *builder) VisitEnumDeclaration(raw phpparser.IEnumDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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

	i, _ := raw.(*phpparser.GlobalConstantDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitClassDeclaration(raw phpparser.IClassDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ClassDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitFunctionDeclaration(raw phpparser.IFunctionDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.FunctionDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitNamespaceDeclaration(raw phpparser.INamespaceDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.NamespaceDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitUseDeclaration(raw phpparser.IUseDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.UseDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitStatement(raw phpparser.IStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.StatementContext)
	if i == nil {
		return nil
	}

	if i.LabelStatement() != nil {
		y.VisitLabelStatement(i.LabelStatement())
	} else if i.BlockStatement() != nil {
		y.VisitBlockStatement(i.BlockStatement())
	} else if i.IfStatement() != nil {
		y.VisitIfStatement(i.IfStatement())
	} else if i.WhileStatement() != nil {
		y.VisitWhileStatement(i.WhileStatement())
	} else if i.DoWhileStatement() != nil {
		y.VisitDoWhileStatement(i.DoWhileStatement())
	} else if i.ForStatement() != nil {
		y.VisitForStatement(i.ForStatement())
	} else if i.SwitchStatement() != nil {
		y.VisitSwitchStatement(i.SwitchStatement())
	} else if i.BreakStatement() != nil {
		y.VisitBreakStatement(i.BreakStatement())
	} else if i.ContinueStatement() != nil {
		y.VisitContinueStatement(i.ContinueStatement())
	} else if i.ReturnStatement() != nil {
		y.VisitReturnStatement(i.ReturnStatement())
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

func (y *builder) VisitLabelStatement(raw phpparser.ILabelStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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
