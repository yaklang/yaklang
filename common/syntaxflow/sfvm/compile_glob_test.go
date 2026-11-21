package sfvm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompileGlobPatternWithDash covers names that contain '-'. SSA value text
// contains them (`Function-strcmp`, `Undefined-a.exec(valid)`), and the lexer
// used to split `*Function-strcmp*` into three tokens.
func TestCompileGlobPatternWithDash(t *testing.T) {
	vm := NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(`$a?{*Function-strcmp*}`)
	require.NoError(t, err)
	require.NotNil(t, frame)
	var glob string
	for _, code := range frame.Codes {
		if strings.Contains(code.String(), "push$glob") {
			glob = code.String()
		}
	}
	require.Contains(t, glob, "*Function-strcmp*", "the whole pattern reaches the search")

	// A pattern must not swallow the arrow of `*->a` or `*-{...}->`.
	for _, rule := range []string{`*->a as $x`, `*-{until: ` + "`*->a`" + `}-> a as $x`} {
		_, err := vm.Compile(rule)
		require.NoError(t, err, rule)
	}
}
