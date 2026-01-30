package compiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireIRContainsInOrder(t *testing.T, ir string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		idx := strings.Index(ir, part)
		require.NotEqualf(t, -1, idx, "expected IR to contain %q", part)
		require.Greaterf(t, idx, last, "expected IR part %q after previous part", part)
		last = idx
	}
}

func TestIR_ObjectInteropCalls(t *testing.T) {
	code := `
	func main() {
		a = getObject(10)
		v = a.Number
		println(v)
	}
	`
	_, _, ir, err := compileToIRFromCode(code, "yak")
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"call ptr @yak_runtime_get_object",
		"call i64 @yak_runtime_get_field",
		"call void @yak_internal_print_int",
	)
}

func TestIR_LoopEmitsBranchesAndCalls(t *testing.T) {
	code := `
	func main() {
		i = 0
		for {
			if i > 3 { break }
			a = getObject(i)
			i = i + 1
		}
		println(999)
	}
	`
	_, _, ir, err := compileToIRFromCode(code, "yak")
	require.NoError(t, err)
	requireIRContainsInOrder(t, ir,
		"br i1",
		"call ptr @yak_runtime_get_object",
		"call void @yak_internal_print_int",
	)
}
