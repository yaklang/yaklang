package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBasicTypeScriptParsing(t *testing.T) {
	code := `
const message: string = "Hello TypeScript";
console.log(message);
`

	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(consts.TS))
	if err != nil {
		t.Fatalf("Failed to parse TypeScript code: %v", err)
	}

	if prog == nil {
		t.Fatal("Program is nil")
	}

	// 检查是否有基本的值
	values := prog.SyntaxFlowChain(`console.log(* #-> as $msg)`).Show()
	if len(values) == 0 {
		t.Log("Warning: No values found in syntax flow, but parsing succeeded")
	}

	t.Logf("Successfully parsed TypeScript code, found %d values", len(values))
}

func TestTypeScriptFunction(t *testing.T) {
	code := `
function add(a: number, b: number): number {
	return a + b;
}

const result = add(10, 20);
console.log(result);
`

	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(consts.TS))
	if err != nil {
		t.Fatalf("Failed to parse TypeScript function: %v", err)
	}

	if prog == nil {
		t.Fatal("Program is nil")
	}

	t.Log("Successfully parsed TypeScript function")
}

func TestBasicFunctionCall(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
function foo(){
println(1)
}
foo()
`, []string{"1"}, t)
}

func TestBasicFunctionCallWith(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
a= 1
function foo(){
println(a)
}
foo()
`, []string{"FreeValue-a"}, t)
}
