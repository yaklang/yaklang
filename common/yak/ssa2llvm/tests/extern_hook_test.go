package tests

import (
	"testing"
)

func TestBinary_CustomExternBindingWithLinkedObject(t *testing.T) {
	code := `
func main() {
    v = getObject(7)
    println(v)
}
`
	goCode := `
import "fmt"

func getObject(x int64) int64 {
	return x * 3
}

func yak_internal_print_int(n int64) {
	fmt.Println(n)
}

func yak_runtime_gc() {}
`

	output := runBinaryWithEnv(t, code, "main", nil, withRuntimeCode(goCode))
	if output != "21\n" {
		t.Fatalf("expected output 21, got %q", output)
	}
}
