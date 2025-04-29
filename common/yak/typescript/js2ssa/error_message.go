package js2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "JS"

// current incomplete implement

func UnexpectedArithmeticOP() string {
	return "unexpected binary arithmetic operator"
}

func UnexpectedBinaryBitWiseOP() string {
	return "unexpected binary bitwise operator"
}

func UnexpectedComparisonOP() string {
	return "unexpected binary comparison operator"
}

func UnexpectedLogicalOP() string {
	return "unexpected binary logical operator"
}

// semantic error

func UnexpectedVariableDeclarationModifierError(name string) string {
	return fmt.Sprintf("unexpected modifier when declare variable: %s", name)
}

func ConstDeclarationWithoutInitializer() string {
	return "const declaration without an initializer"
}
func BindPatternDeclarationWithoutInitializer() string {
	return "binding pattern declaration without an initializer"
}

func NoDeclaraionName() string {
	return "no declaration name found"
}

func RestElementRequiresIdentifier() string {
	return "rest element requires identifier"
}

func InvalidPropertyBinding() string {
	return "invalid property binding"
}
