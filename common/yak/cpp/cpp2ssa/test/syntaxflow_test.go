package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestCppSyntaxFlowVariableDef covers the basic def-use rule: a local assigned
// from a function return resolves to the returned constant.
func TestCppSyntaxFlowVariableDef(t *testing.T) {
	test.CheckSyntaxFlow(t, `
int source() {
    return 7;
}

int main() {
    int value = source();
}
`, `
value #-> as $value_def
`, map[string][]string{
		"value_def": {"7"},
	}, ssaapi.WithLanguage(ssaconfig.CPP))
}

// TestCppSyntaxFlowCallArgument covers cross-function dataflow: the argument of
// sink is traced back to its constant definition.
func TestCppSyntaxFlowCallArgument(t *testing.T) {
	test.CheckSyntaxFlow(t, `
int sink(int value) {
    return value;
}

int main() {
    int source = 7;
    sink(source);
}
`, `
sink(* as $arg)
$arg #-> as $arg_def
`, map[string][]string{
		"arg_def": {"7"},
	}, ssaapi.WithLanguage(ssaconfig.CPP))
}

// TestCppSyntaxFlowReturnChain covers an advanced cross-function chain with a
// branch phi: the returned values of pick reach the caller.
func TestCppSyntaxFlowReturnChain(t *testing.T) {
	test.CheckSyntaxFlowContain(t, `
int pick(int value) {
    if (value > 0) {
        return 1;
    }
    return 0;
}

int main() {
    int result = pick(2);
}
`, `
result #-> as $result_def
`, map[string][]string{
		"result_def": {"1", "0"},
	}, ssaapi.WithLanguage(ssaconfig.CPP))
}
