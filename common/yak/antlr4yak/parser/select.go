package parser

import (
	"fmt"

	"github.com/yaklang/antlr/v4"
)

func selectHasFallthrough(tree antlr.Tree) bool {
	switch tree.(type) {
	case *SwitchStmtContext, *AnonymousFunctionDeclContext, *InstanceCodeContext:
		return false
	case *FallthroughStmtContext:
		return true
	}
	for _, child := range tree.GetChildren() {
		if selectHasFallthrough(child) {
			return true
		}
	}
	return false
}

// IsSelectStatement recognizes select without reserving a new lexer token.
func (p *YaklangParser) IsSelectStatement() bool {
	s := p.GetTokenStream()
	return s.LA(1) == YaklangParserIdentifier && s.LT(1).GetText() == "select" && s.LA(2) == YaklangParserLBrace
}

// SelectCommunication describes operands, not an operation to execute yet.
// A nil Channel denotes default. Left is evaluated only in the chosen case.
type SelectCommunication struct {
	Channel IExpressionContext
	Send    IExpressionContext
	Left    ILeftExpressionListContext
	Declare bool
}

func selectReceive(raw IExpressionContext) IExpressionContext {
	e, _ := raw.(*ExpressionContext)
	for e != nil && e.ParenExpression() != nil {
		e, _ = e.ParenExpression().(*ParenExpressionContext).Expression().(*ExpressionContext)
	}
	if e != nil && e.UnaryOperator() != nil && e.UnaryOperator().GetText() == "<-" {
		return e.Expression(0)
	}
	return nil
}

// SelectCommunications validates the same communication shapes in both frontends.
func SelectCommunications(stmt *SelectStmtContext) ([]SelectCommunication, error) {
	clauses := stmt.AllSelectClause()
	cases := make([]SelectCommunication, 0, len(clauses))
	defaultSeen := false
	for _, raw := range clauses {
		clause := raw.(*SelectClauseContext)
		if selectHasFallthrough(clause) {
			return nil, fmt.Errorf("fallthrough is not allowed in select")
		}
		c := SelectCommunication{}
		if clause.Default() != nil {
			if defaultSeen {
				return nil, fmt.Errorf("select has multiple default clauses")
			}
			defaultSeen = true
		} else {
			comm := clause.SelectComm().(*SelectCommContext)
			if raw := comm.AssignExpression(); raw != nil {
				a := raw.(*AssignExpressionContext)
				if a.ExpressionList() == nil || (a.AssignEq() == nil && a.ColonAssignEq() == nil) {
					return nil, fmt.Errorf("select case must be a channel send or receive")
				}
				exprs := a.ExpressionList().(*ExpressionListContext).AllExpression()
				if len(exprs) != 1 {
					return nil, fmt.Errorf("select receive requires one receive expression")
				}
				c.Channel = selectReceive(exprs[0])
				c.Left, c.Declare = a.LeftExpressionList(), a.ColonAssignEq() != nil
				lefts := c.Left.(*LeftExpressionListContext).AllLeftExpression()
				if len(lefts) < 1 || len(lefts) > 2 {
					return nil, fmt.Errorf("select receive assigns to one or two variables")
				}
				if c.Declare {
					for _, left := range lefts {
						if left.(*LeftExpressionContext).Identifier() == nil {
							return nil, fmt.Errorf("select receive declaration requires identifiers")
						}
					}
				}
			} else {
				e := comm.Expression().(*ExpressionContext)
				if e.ChanIn() != nil {
					c.Channel, c.Send = e.Expression(0), e.Expression(1)
				} else {
					c.Channel = selectReceive(e)
				}
			}
			if c.Channel == nil {
				return nil, fmt.Errorf("select case must be a channel send or receive")
			}
		}
		cases = append(cases, c)
	}
	return cases, nil
}
