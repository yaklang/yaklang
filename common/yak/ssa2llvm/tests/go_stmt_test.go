package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoStmt_PrintlnWait(t *testing.T) {
	code := `
func main() {
	go println(1)
	waitAllAsyncCallFinish()
	println(2)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "1\n2\n", output)
}

func TestGoStmt_DirectFunctionCall(t *testing.T) {
	code := `
func f(x) {
	println(x)
}

func main() {
	go f(10)
	waitAllAsyncCallFinish()
	println(20)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "10\n20\n", output)
}

func TestGoStmt_MethodCall_ThisBinding(t *testing.T) {
	code := `
func main() {
	f = () => {
		this = {
			"key": 1,
			"set": (i) => { this.key = i },
			"get": () => this.key,
		}
		return this
	}
	a = f()
	go a.set(2)
	waitAllAsyncCallFinish()
	println(a.get())
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "2\n", output)
}
