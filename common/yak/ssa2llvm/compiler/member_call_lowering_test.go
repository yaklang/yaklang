package compiler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemberCallLowering_CrossBlockMemberReadCompiles(t *testing.T) {
	// Regression for dominance issues when object materialization and member access
	// were emitted in different blocks (seen in Shiro coreplugins).
	code := `
check = () => {
	obj = {"k": 1}
	if true {
		obj = {"k": 2}
	}
	return obj.k
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	requireIRContainsSlotLowering(t, ir)
	require.Contains(t, ir, "call i64 @")
}

func TestMemberCallLowering_UndefinedMemberOnMapCompiles(t *testing.T) {
	code := `
check = () => {
	m = {"a": 1}
	return m.missing
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, "call i64 @")
}

func TestMemberCallLowering_LazyCallResultSlotStoreDominatesUse(t *testing.T) {
	code := `
check = () => {
	raw = "/api/v1"
	trimmed = raw.TrimLeft("/")
	parts = str.Split(trimmed, "/")
	first = parts[0]
	return len(first)
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, "yak_method_dispatch_ctx")
	require.Contains(t, ir, "yak_yaklib_ctx")
	requireIRContainsSlotLowering(t, ir)
}
