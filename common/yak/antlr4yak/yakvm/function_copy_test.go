package yakvm

import "testing"

func TestFunctionCopyPreservesRuntimeMetadata(t *testing.T) {
	vm := New()
	definitionFrame := NewFrame(vm)
	function := NewFunction(nil, vm.rootScope.GetSymTable())
	function.defineFrame = definitionFrame
	function.FreeValue = []int{3, 7}

	copied := function.Copy(vm.rootScope)
	if copied == function {
		t.Fatal("Function.Copy returned the original function")
	}
	if copied.defineFrame != definitionFrame {
		t.Fatal("Function.Copy dropped the sandbox definition frame")
	}
	if len(copied.FreeValue) != 2 || copied.FreeValue[0] != 3 || copied.FreeValue[1] != 7 {
		t.Fatalf("Function.Copy lost free-value metadata: %v", copied.FreeValue)
	}
}
