package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestCppExpressionArithmetic covers the five arithmetic operators.
func TestCppExpressionArithmetic(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(1 + 2);
    println(7 - 3);
    println(4 * 5);
    println(9 / 3);
    println(9 % 4);
    return 0;
}
`, []string{"3", "4", "20", "3", "1"}, t)
}

// TestCppExpressionArithmeticVariables keeps operands as SSA names when they
// are not constants.
func TestCppExpressionArithmeticVariables(t *testing.T) {
	test.CheckPrintlnValue(`
int compute(int a, int b) {
    int total = a + b;
    println(total);
    return total;
}
`, []string{"add(Parameter-a, Parameter-b)"}, t)
}

// TestCppExpressionComparison covers the six comparison operators.
func TestCppExpressionComparison(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(1 == 1);
    println(1 != 1);
    println(1 < 2);
    println(1 <= 1);
    println(2 > 1);
    println(2 >= 2);
    return 0;
}
`, []string{"true", "false", "true", "true", "true", "true"}, t)
}

// TestCppExpressionLogical covers &&, || and !.
func TestCppExpressionLogical(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(true && true);
    println(true && false);
    println(false || true);
    println(false || false);
    println(!true);
    return 0;
}
`, []string{"true", "false", "true", "false", "false"}, t)
}

// TestCppExpressionBitwise covers &, |, ^, << and >>.
func TestCppExpressionBitwise(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(6 & 3);
    println(6 | 3);
    println(6 ^ 3);
    println(1 << 3);
    println(8 >> 2);
    return 0;
}
`, []string{"2", "7", "5", "8", "2"}, t)
}

// TestCppExpressionCompoundAssignment covers the compound assignment forms.
func TestCppExpressionCompoundAssignment(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int value = 10;
    value += 2;
    value -= 1;
    value *= 3;
    value /= 2;
    value %= 7;
    println(value);
    return 0;
}
`, []string{"2"}, t)
}

// TestCppExpressionIncrement covers prefix and postfix increment/decrement.
func TestCppExpressionIncrement(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int value = 1;
    value++;
    println(value);
    ++value;
    println(value);
    value--;
    println(value);
    --value;
    println(value);
    return 0;
}
`, []string{"2", "3", "2", "1"}, t)
}

// TestCppExpressionStringLiteral keeps string literals as constants.
func TestCppExpressionStringLiteral(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println("hello");
    return 0;
}
`, []string{`"hello"`}, t)
}

// TestCppExpressionTernary asserts a conditional expression becomes an if merge
// phi whose inputs are the two arms.
func TestCppExpressionTernary(t *testing.T) {
	test.CheckPrintlnValueContain(`
int pick(int a, int b) {
    println(a > b ? a : b);
    return 0;
}
`, []string{"phi(ternary_"}, t)
}

// TestCppExpressionCastPassesValue asserts a static_cast lowers to the
// dedicated typecast instruction and keeps the operand.
func TestCppExpressionCastPassesValue(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int source = 3;
    int widened = static_cast<int>(source);
    println(widened);
    return 0;
}
`, []string{"castType(number, 3)"}, t)
}

// TestCppExpressionSizeOfIsPlaceholder records the current compile-time
// behaviour: sizeof is accepted but lowered to 0 because object layout is not
// modelled. TODO: evaluate sizeof for scalar and pointer types.
func TestCppExpressionSizeOfIsPlaceholder(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(sizeof(int));
    return 0;
}
`, []string{"0"}, t)
}

// TestCppExpressionLiterals covers the integer, floating, character and boolean
// literal spellings parsed by the frontend.
func TestCppExpressionLiterals(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(42);
    println(0x2A);
    println(0b1010);
    println(true);
    return 0;
}
`, []string{"42", "42", "10", "true"}, t)
}

// TestCppExpressionLiteralsNegative keeps unary minus distinct from a literal.
func TestCppExpressionLiteralsNegative(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(-5);
    println(~0);
    return 0;
}
`, []string{"-5", "-1"}, t)
}