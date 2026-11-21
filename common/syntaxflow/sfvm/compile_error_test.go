package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompileErrorPointsAtTheProblem replaces the raw ANTLR token dump
// (`symbol: [@36,110:116='strcmp*',<88>,4:17]`) with the rule line, a caret,
// and a hint that names the expected syntax.
func TestCompileErrorPointsAtTheProblem(t *testing.T) {
	_, err := NewSyntaxFlowVirtualMachine().Compile("$a?{name: strcmp}")
	require.Error(t, err)
	msg := err.Error()
	require.NotContains(t, msg, "symbol: [@", "no token index dump")
	require.Contains(t, msg, "^", "the caret marks the column")
	require.Contains(t, msg, "hint: ", "the message says what was expected")
	require.Contains(t, msg, "1 | $a?{name: strcmp}")
	require.Contains(t, msg, "unknown condition key 'name:'",
		"a key the parser does not know is named in the hint")
}

// TestCompileErrorHintForBareDashInCondition checks the hint that explains why
// a '-' inside ?{} needs the glob form.
func TestCompileErrorHintForBareDashInCondition(t *testing.T) {
	// `Function-strcmp` without a leading '*' is an identifier followed by '-'.
	_, err := NewSyntaxFlowVirtualMachine().Compile("$a?{Function-strcmp}")
	require.Error(t, err)
	require.Contains(t, err.Error(), "hint: ")
}
