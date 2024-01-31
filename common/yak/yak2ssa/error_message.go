package yak2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "yakast"

func MultipleAssignFailed(left, right int) string {
	return fmt.Sprintf("multi-assign failed: left value length[%d] != right value length[%d]", left, right)
}

func AssignLeftSideEmpty() string {
	return "assign left side is empty"
}

func AssignRightSideEmpty() string {
	return "assign right side is empty"
}

func UnaryOperatorNotSupport(op string) string {
	return fmt.Sprintf("unary operator not support: %s", op)
}
func BinaryOperatorNotSupport(op string) string {
	return fmt.Sprintf("binary operator not support: %s", op)
}

func ArrowFunctionNeedExpressionOrBlock() string {
	return "BUG: arrow function need expression or block at least"
}

func ExpressionNotVariable(expr string) string {
	return fmt.Sprintf("Expression: %s is not a variable", expr)
}

func UnexpectedBreakStmt() string {
	return "unexpected break stmt"
}

func UnexpectedContinueStmt() string {
	return "unexpected continue stmt"
}

func UnexpectedFallthroughStmt() string {
	return "unexpected fallthrough stmt"
}

func UnexpectedAssertStmt() string {
	return "unexpected assert stmt, this not expression"
}

func SliceCallExpressionTooMuch() string {
	return "slice call expression too much"
}

func SliceCallExpressionIsEmpty() string {
	return "slice call expression is empty"
}

func MakeArgumentTooMuch(typ string) string {
	return fmt.Sprintf("make %s expression argument too much!", typ)
}

func NotSetTypeInMakeExpression() string {
	return "not set type in make expression"
}

func MakeUnknownType() string {
	return "make unknown type"
}

func FieldCallTargetError(name string) string {
	return fmt.Sprintf("%s call target Error", name)
}
