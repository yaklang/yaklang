package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvoke_MainWrapperPrintEntryResult(t *testing.T) {
	code := `
check = () => {
	return 42
}
`
	exitCode, output := runBinaryExitCodeWithEnv(t, code, "check", nil, withCompilePrintEntryResult(true))
	require.Equal(t, 42, exitCode)
	require.Equal(t, "42\n", output)
}

func TestInvoke_StdlibGetenv(t *testing.T) {
	code := `
func main() {
	println(os.Getenv("YAK_SSA2LLVM_INVOKE"))
}
`
	output := runBinaryWithEnv(t, code, "main", map[string]string{
		"YAK_SSA2LLVM_INVOKE": "invoke-ok",
	})
	require.Equal(t, "invoke-ok\n", output)
}

func TestInvoke_CallableAndDispatchCompose(t *testing.T) {
	code := `
add = (a, b) => {
	return a + b
}

func main() {
	println(add(10, 32))
	println(os.Getenv("YAK_SSA2LLVM_COMPOSE"))
}
`
	output := runBinaryWithEnv(t, code, "main", map[string]string{
		"YAK_SSA2LLVM_COMPOSE": "dispatch-ok",
	})
	require.Equal(t, "42\ndispatch-ok\n", output)
}
