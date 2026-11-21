package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompileOpcodeNameValidation turns a silent empty result into a compile
// error. `?{opcode: ge}` used to compile, log "invalid opcode" at INFO level,
// and match nothing, which reads exactly like "the program has no such value".
func TestCompileOpcodeNameValidation(t *testing.T) {
	_, err := NewSyntaxFlowVirtualMachine().Compile(`$a?{opcode: ge}`)
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "unknown opcode name(s) 'ge'")
	require.Contains(t, msg, "gteq", "the error lists the accepted spellings")

	// Valid spellings from both tables still compile. Only name spellings can
	// appear here: the filter takes an identifier, so "gteq" parses and ">="
	// does not.
	for _, name := range []string{"call", "phi", "if", "eq", "gteq", "lteq", "const"} {
		_, err := NewSyntaxFlowVirtualMachine().Compile(`$a?{opcode: ` + name + `}`)
		require.NoError(t, err, name)
	}

	// One bad name among good ones still fails.
	_, err = NewSyntaxFlowVirtualMachine().Compile(`$a?{opcode: call, nope}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "'nope'")
	require.NotContains(t, err.Error(), "'call'")
}
