package yak

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	yakparser "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

type hotPatchStaticValueKind uint8

const (
	hotPatchStaticValueOther hotPatchStaticValueKind = iota
	hotPatchStaticValueFunction
)

type hotPatchHookUsage struct {
	legacy bool
	phase  bool
}

type hotPatchStaticScope struct {
	bindings map[string]hotPatchStaticValueKind
}

func detectHotPatchHookUsage(code string) (hotPatchHookUsage, error) {
	program, err := parseHotPatchProgram(code)
	if err != nil {
		return hotPatchHookUsage{}, err
	}
	scope := &hotPatchStaticScope{bindings: make(map[string]hotPatchStaticValueKind)}
	scope.applyProgram(program)
	return scope.snapshotUsage(), nil
}

func parseHotPatchProgram(code string) (*yakparser.ProgramContext, error) {
	errListener := &hotPatchSyntaxErrorListener{}
	input := antlr.NewInputStream(code)
	lexer := yakparser.NewYaklangLexer(input)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)

	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yakparser.NewYaklangParser(tokens)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)

	program, _ := parser.Program().(*yakparser.ProgramContext)
	if errListener.err != nil {
		return nil, errListener.err
	}
	if program == nil {
		return nil, utils.Error("parse hotpatch code failed: empty program")
	}
	return program, nil
}

func (s *hotPatchStaticScope) applyProgram(program *yakparser.ProgramContext) {
	if s == nil || program == nil {
		return
	}
	stmtList, _ := program.StatementList().(*yakparser.StatementListContext)
	if stmtList == nil {
		return
	}
	for _, raw := range stmtList.AllStatement() {
		stmt, _ := raw.(*yakparser.StatementContext)
		s.applyStatement(stmt)
	}
}

func (s *hotPatchStaticScope) applyStatement(stmt *yakparser.StatementContext) {
	if stmt == nil {
		return
	}
	if assignStmt, _ := stmt.AssignExpressionStmt().(*yakparser.AssignExpressionStmtContext); assignStmt != nil {
		assignExpr, _ := assignStmt.AssignExpression().(*yakparser.AssignExpressionContext)
		s.applyAssignExpression(assignExpr)
		return
	}
	declareStmt, _ := stmt.DeclareVariableExpressionStmt().(*yakparser.DeclareVariableExpressionStmtContext)
	if declareStmt == nil {
		return
	}
	declareExpr, _ := declareStmt.DeclareVariableExpression().(*yakparser.DeclareVariableExpressionContext)
	if declareExpr == nil {
		return
	}
	assignExpr, _ := declareExpr.DeclareAndAssignExpression().(*yakparser.DeclareAndAssignExpressionContext)
	s.applyDeclareAndAssignExpression(assignExpr)
}

func (s *hotPatchStaticScope) applyAssignExpression(expr *yakparser.AssignExpressionContext) {
	if expr == nil {
		return
	}
	lefts, _ := expr.LeftExpressionList().(*yakparser.LeftExpressionListContext)
	rights, _ := expr.ExpressionList().(*yakparser.ExpressionListContext)
	s.applyBindingList(lefts, rights)
}

func (s *hotPatchStaticScope) applyDeclareAndAssignExpression(expr *yakparser.DeclareAndAssignExpressionContext) {
	if expr == nil {
		return
	}
	lefts, _ := expr.LeftExpressionList().(*yakparser.LeftExpressionListContext)
	rights, _ := expr.ExpressionList().(*yakparser.ExpressionListContext)
	s.applyBindingList(lefts, rights)
}

func (s *hotPatchStaticScope) applyBindingList(lefts *yakparser.LeftExpressionListContext, rights *yakparser.ExpressionListContext) {
	names := hotPatchAssignedNames(lefts)
	kinds := s.expressionKinds(rights)
	if len(names) == 0 {
		return
	}
	if len(names) != len(kinds) {
		for _, name := range names {
			s.bindings[name] = hotPatchStaticValueOther
		}
		return
	}
	for i, name := range names {
		if name == "" {
			continue
		}
		s.bindings[name] = kinds[i]
	}
}

func hotPatchAssignedNames(lefts *yakparser.LeftExpressionListContext) []string {
	if lefts == nil {
		return nil
	}
	rawLefts := lefts.AllLeftExpression()
	names := make([]string, 0, len(rawLefts))
	for _, raw := range rawLefts {
		left, _ := raw.(*yakparser.LeftExpressionContext)
		if left == nil || left.Identifier() == nil {
			names = append(names, "")
			continue
		}
		names = append(names, left.Identifier().GetText())
	}
	return names
}

func (s *hotPatchStaticScope) expressionKinds(rights *yakparser.ExpressionListContext) []hotPatchStaticValueKind {
	if rights == nil {
		return nil
	}
	rawExprs := rights.AllExpression()
	kinds := make([]hotPatchStaticValueKind, 0, len(rawExprs))
	for _, raw := range rawExprs {
		expr, _ := raw.(*yakparser.ExpressionContext)
		kinds = append(kinds, s.expressionKind(expr))
	}
	return kinds
}

func (s *hotPatchStaticScope) expressionKind(expr *yakparser.ExpressionContext) hotPatchStaticValueKind {
	if expr == nil {
		return hotPatchStaticValueOther
	}
	if expr.AnonymousFunctionDecl() != nil {
		return hotPatchStaticValueFunction
	}
	if paren, _ := expr.ParenExpression().(*yakparser.ParenExpressionContext); paren != nil {
		inner, _ := paren.Expression().(*yakparser.ExpressionContext)
		return s.expressionKind(inner)
	}
	if expr.Identifier() == nil {
		return hotPatchStaticValueOther
	}
	if s.bindings[expr.Identifier().GetText()] == hotPatchStaticValueFunction {
		return hotPatchStaticValueFunction
	}
	return hotPatchStaticValueOther
}

func (s *hotPatchStaticScope) snapshotUsage() hotPatchHookUsage {
	return hotPatchHookUsage{
		legacy: s.hasFunctionBinding(legacyHotPatchHooks),
		phase:  s.hasFunctionBinding(hotPatchPhaseHooks),
	}
}

func (s *hotPatchStaticScope) hasFunctionBinding(hooks []string) bool {
	for _, name := range hooks {
		if s.bindings[name] == hotPatchStaticValueFunction {
			return true
		}
	}
	return false
}

type hotPatchSyntaxErrorListener struct {
	antlr.DefaultErrorListener
	err error
}

func (l *hotPatchSyntaxErrorListener) SyntaxError(
	_ antlr.Recognizer,
	_ interface{},
	line int,
	column int,
	msg string,
	_ antlr.RecognitionException,
) {
	if l.err != nil {
		return
	}
	l.err = utils.Errorf("parse hotpatch code failed at %d:%d: %s", line, column, msg)
}
