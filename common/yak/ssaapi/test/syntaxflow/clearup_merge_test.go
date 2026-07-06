package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// Focused guards for the clearup merge-skip path (ssaapi/sf_config.go clearup
// + sfvm.SymbolSnapshot). The red→green validation for the Opt A over-skip fix
// is the 11 pre-existing dataflow tests that pass on clean main and fail on Opt
// A (980f0cd14): Test_TopDef_UD_Relationship, Test_Bottom_Use_UD_Relationship,
// TestDataflowTest, Test_Include_WithGraph, TestSF_Config_MultipleConfig,
// TestSSARisk_Normal, TestSyntacticSugar_ConstInRecursive, and 4 buildin
// TestVerifiedRule Java rules. This file adds small correctness guards so the
// snapshot-delta fix doesn't regress the cases it must preserve.

// TestClearupMerge_PositionalDataflowNamedVarSurfaces guards that a NAMED
// variable bound inside a positional dataflow(<<<CODE ... CODE>) sub-rule
// surfaces in the result. This case passes on BOTH main and Opt A (the
// positional dataflow CODE is evaluated as the dataflow result, not as an
// include-filter CheckMatch); the snapshot-delta fix must keep it green.
func TestClearupMerge_PositionalDataflowNamedVarSurfaces(t *testing.T) {
	code := `
f2 := func(param2) { exec(param2) }
f1 := func(param1) { f2(param1) }
`
	rule := `
exec(* as $sink);
param1?{opcode:param} as $source;
$sink #-> as $result;
$result<dataflow(<<<CODE
<self> & $sink as $start;
<self> & $source as $end;
CODE)>
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		vals, err := prog.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.NotEmpty(t, vals.GetValues("start"),
			"$start (named var in positional dataflow CODE) must surface")
		require.NotEmpty(t, vals.GetValues("end"),
			"$end (named var in positional dataflow CODE) must surface")
		return nil
	})
}