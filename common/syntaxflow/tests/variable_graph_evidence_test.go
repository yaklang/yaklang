package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func TestVariableGraphEvidenceAttach(t *testing.T) {
	tests := []struct {
		VarGraphTestCase
		Edges []EdgeAssertion
	}{
		// ==================== 搜索语句证据测试 ====================
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "exact search evidence - name match",
				Code:   `source = "hello"`,
				SFRule: `source as $source;`,
			},
			Edges: []EdgeAssertion{
				{
					From: "", To: "source", StepCount: 1,
					Steps: []StepAssertion{{
						StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, HasDescZh: true,
						HasValues: true, DescContains: "Exact Search",
						HasSearchMode: true, SearchMatchMode: sfvm.NameMatch | sfvm.KeyMatch,
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "exact search evidence - key match (member access)",
				Code:   `a = {"source": "hello"}`,
				SFRule: `.source as $source;`,
			},
			Edges: []EdgeAssertion{
				{
					From: "", To: "source", StepCount: 1,
					Steps: []StepAssertion{{
						StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, HasDescZh: true,
						HasValues: true, DescContains: "Exact Search",
						HasSearchMode: true, SearchMatchMode: sfvm.KeyMatch,
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "fuzzy search evidence",
				Code:   `a1 = "hello"; a2 = "world"`,
				SFRule: `a* as $a;`,
			},
			Edges: []EdgeAssertion{
				{
					From: "", To: "a", StepCount: 1,
					Steps: []StepAssertion{{
						StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, HasDescZh: true,
						HasValues: true, DescContains: "Fuzzy Search",
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "regexp search evidence",
				Code:   `abc = "hello"; xyz = "world"`,
				SFRule: `/a.*/ as $a;`,
			},
			Edges: []EdgeAssertion{
				{
					From: "", To: "a", StepCount: 1,
					Steps: []StepAssertion{{
						StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, HasDescZh: true,
						HasValues: true, DescContains: "Regexp Search",
					}},
				},
			},
		},
		// ==================== 数据流语句证据测试 ====================
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "data flow get users evidence",
				Code:   `a = 1; b = a + 2; c = b * 3`,
				SFRule: `a as $source; $source-> as $sink;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "source",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeSearch,
						HasDesc:      true,
						DescContains: "Exact Search",
					}},
				},
				{
					From:      "source",
					To:        "sink",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeDataFlow,
						HasDesc:      true,
						HasDescZh:    true,
						HasValues:    true,
						DescContains: "Get Users",
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "data flow top defs evidence",
				Code:   `a = 1; b = a + 2; c = b * 3`,
				SFRule: `c as $sink; $sink #-> as $source;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "sink",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeSearch,
						HasDesc:      true,
						DescContains: "Exact Search",
					}},
				},
				{
					From:      "sink",
					To:        "source",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeDataFlow,
						HasDesc:      true,
						HasDescZh:    true,
						HasValues:    true,
						DescContains: "Get TopDefs",
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "data flow with include config",
				Code:   "a = 1\nb = 2\nc = a + b",
				SFRule: `c as $sink; $sink#{include: ` + "`* ?{opcode:const}`" + `}-> as $source;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "sink",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeSearch,
						HasDesc:      true,
						DescContains: "Exact Search",
					}},
				},
				{
					From:      "sink",
					To:        "source",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType: sfvm.AnalysisStepTypeDataFlow, HasDesc: true, HasDescZh: true, HasValues: true, DescContains: "Get TopDefs", HasDataFlow: true,
						DataFlowCheck: func(t *testing.T, mode *sfvm.DataFlowMode) {
							require.Equal(t, sfvm.DataFlowDirectionTopDef, mode.Direction)
							require.NotEmpty(t, mode.Config["include"], "should have include config")
						},
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "data flow with exclude config",
				Code:   "a = 1\nb = a.destroy()\nc = b",
				SFRule: `c as $sink; $sink#{exclude: ` + "`<self>.destroy*`" + `}-> as $source;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "sink",
					StepCount: 1,
					Steps:     []StepAssertion{{StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, DescContains: "Exact Search"}},
				},
				{
					From:      "sink",
					To:        "source",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeDataFlow,
						HasDesc:      true,
						HasDescZh:    true,
						DescContains: "Get TopDefs",
						HasDataFlow:  true,
						DataFlowCheck: func(t *testing.T, mode *sfvm.DataFlowMode) {
							require.Equal(t, sfvm.DataFlowDirectionTopDef, mode.Direction)
							require.NotEmpty(t, mode.Config["exclude"], "should have exclude config")
						},
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "data flow get users evidence 2",
				Code:   "a = 1\nb = a + 2\nc = b * 3",
				SFRule: `a as $source; $source-> as $sink;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "source",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeSearch,
						HasDesc:      true,
						DescContains: "Exact Search",
					}},
				},
				{
					From:      "source",
					To:        "sink",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeDataFlow,
						HasDesc:      true,
						HasDescZh:    true,
						HasValues:    true,
						DescContains: "Get Users",
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "data flow bottom users evidence",
				Code:   "a = 1\nb = a + 2\nc = b * 3",
				SFRule: `a as $source; $source--> as $sink;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "source",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType:     sfvm.AnalysisStepTypeSearch,
						HasDesc:      true,
						DescContains: "Exact Search",
					}},
				},
				{
					From:      "source",
					To:        "sink",
					StepCount: 1,
					Steps: []StepAssertion{{
						StepType: sfvm.AnalysisStepTypeDataFlow, HasDesc: true, HasDescZh: true, HasValues: true, DescContains: "Get BottomUse", HasDataFlow: true,
						DataFlowCheck: func(t *testing.T, mode *sfvm.DataFlowMode) {
							require.Equal(t, sfvm.DataFlowDirectionBottomUse, mode.Direction)
						},
					}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "native call getUsers evidence",
				Code:   "function target(x) { return x * 2 }\na = 10\nb = target(a)",
				SFRule: `a as $var_a; $var_a<getUsers> as $var_b;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "var_a",
					StepCount: 1,
					Steps:     []StepAssertion{{StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, DescContains: "Exact Search"}},
				},
				{
					From:      "var_a",
					To:        "var_b",
					StepCount: 1,
					Steps:     []StepAssertion{{StepType: sfvm.AnalysisStepTypeTransform, HasDesc: true, HasDescZh: true, HasValues: true, DescContains: "Native Call"}},
				},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "native call string transform evidence",
				Code:   `a = "hello"`,
				SFRule: `a as $a; $a<string> as $b;`,
			},
			Edges: []EdgeAssertion{
				{
					From:      "",
					To:        "a",
					StepCount: 1,
					Steps:     []StepAssertion{{StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, DescContains: "Exact Search"}},
				},
				{From: "a", To: "b", StepCount: 1, Steps: []StepAssertion{{StepType: sfvm.AnalysisStepTypeTransform, HasDesc: true, HasDescZh: true, HasValues: true, DescContains: "Native Call"}}},
			},
		},
		// ==================== 过滤语句证据测试 ====================
		{
			VarGraphTestCase: VarGraphTestCase{Name: "simple string filter evidence", Code: "a1 = \"hello\"\na2 = \"world\"", SFRule: `a* ?{have: "hello"} as $sink;`},
			Edges: []EdgeAssertion{{From: "", To: "sink", StepCount: 2, Steps: []StepAssertion{
				{
					StepType:     sfvm.AnalysisStepTypeSearch,
					HasDesc:      true,
					DescContains: "Fuzzy Search",
				},
				{StepType: sfvm.AnalysisStepTypeConditionFilter, HasEvidenceTree: true, EvidenceNodeType: sfvm.EvidenceTypeStringCondition, FilterType: "string", HasPassedValues: true, HasFailedValues: true},
			}}},
		},
		{
			VarGraphTestCase: VarGraphTestCase{Name: "opcode filter evidence", Code: "a1 = \"hello\"\na2 = \"world\"", SFRule: `a* ?{opcode: const} as $sink;`},
			Edges: []EdgeAssertion{{From: "", To: "sink", StepCount: 2, Steps: []StepAssertion{
				{
					StepType:     sfvm.AnalysisStepTypeSearch,
					HasDesc:      true,
					DescContains: "Fuzzy Search",
				},
				{StepType: sfvm.AnalysisStepTypeConditionFilter, HasEvidenceTree: true, EvidenceNodeType: sfvm.EvidenceTypeOpcodeCondition, FilterType: "opcode", HasPassedValues: true},
			}}},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "logic AND evidence",
				Code:   "a1 = \"hello\"\na2 = \"world\"",
				SFRule: `a* ?{have: "hello" && opcode: const} as $sink;`,
			},
			Edges: []EdgeAssertion{{From: "", To: "sink", StepCount: 2, Steps: []StepAssertion{
				{
					StepType:     sfvm.AnalysisStepTypeSearch,
					HasDesc:      true,
					DescContains: "Fuzzy Search",
				},
				{StepType: sfvm.AnalysisStepTypeConditionFilter, HasEvidenceTree: true, EvidenceNodeType: sfvm.EvidenceTypeLogicGate, LogicOp: sfvm.ConditionTypeAnd, ChildrenCount: 2},
			}}},
		},
		{VarGraphTestCase: VarGraphTestCase{
			Name:   "logic OR evidence",
			Code:   "a1 = \"hello\"\na2 = \"world\"",
			SFRule: `a* ?{have: "hello" || have: "world"} as $sink;`,
		}, Edges: []EdgeAssertion{{From: "", To: "sink", StepCount: 2, Steps: []StepAssertion{
			{
				StepType:     sfvm.AnalysisStepTypeSearch,
				HasDesc:      true,
				DescContains: "Fuzzy Search",
			},
			{
				StepType:         sfvm.AnalysisStepTypeConditionFilter,
				HasEvidenceTree:  true,
				EvidenceNodeType: sfvm.EvidenceTypeLogicGate,
				LogicOp:          sfvm.ConditionTypeOr,
				ChildrenCount:    2,
			},
		}}}},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "logic NOT evidence",
				Code:   "a1 = \"hello\"\na2 = \"world\"",
				SFRule: `a* ?{!have: "hello"} as $sink;`,
			},
			Edges: []EdgeAssertion{{
				From:      "",
				To:        "sink",
				StepCount: 2,
				Steps: []StepAssertion{
					{StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, DescContains: "Fuzzy Search"},
					{
						StepType:         sfvm.AnalysisStepTypeConditionFilter,
						HasEvidenceTree:  true,
						EvidenceNodeType: sfvm.EvidenceTypeLogicGate,
						LogicOp:          sfvm.ConditionTypeNot,
						ChildrenCount:    1,
					},
				},
			}},
		},
		{
			VarGraphTestCase: VarGraphTestCase{Name: "chain filter evidence - multiple steps", Code: "a1 = \"hello\"\na2 = \"world\"", SFRule: `a* ?{have: "o"}?{opcode: const} as $sink;`},
			Edges: []EdgeAssertion{{From: "", To: "sink", StepCount: 3, Steps: []StepAssertion{
				{
					StepType:     sfvm.AnalysisStepTypeSearch,
					HasDesc:      true,
					DescContains: "Fuzzy Search",
				},
				{
					StepType:         sfvm.AnalysisStepTypeConditionFilter,
					HasEvidenceTree:  true,
					EvidenceNodeType: sfvm.EvidenceTypeStringCondition,
					FilterType:       "string",
				},
				{StepType: sfvm.AnalysisStepTypeConditionFilter, HasEvidenceTree: true, EvidenceNodeType: sfvm.EvidenceTypeOpcodeCondition, FilterType: "opcode"},
			}}},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "complex logic: (A && B) || C",
				Code:   "a1 = \"hello\"\na2 = \"world\"\na3 = \"foo\"",
				SFRule: `a* ?{(have: "hello" && opcode: const) || have: "world"} as $sink;`,
			},
			Edges: []EdgeAssertion{{From: "", To: "sink", StepCount: 2, Steps: []StepAssertion{
				{StepType: sfvm.AnalysisStepTypeSearch, HasDesc: true, DescContains: "Fuzzy Search"},
				{
					StepType:         sfvm.AnalysisStepTypeConditionFilter,
					HasEvidenceTree:  true,
					EvidenceNodeType: sfvm.EvidenceTypeLogicGate,
					LogicOp:          sfvm.ConditionTypeOr,
					ChildrenCount:    2,
				},
			}}},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "filter with source variable",
				Code:   "a1 = \"hello\"\na2 = \"world\"",
				SFRule: "a* as $a;\n$a?{have: \"hello\"} as $b;",
			},
			Edges: []EdgeAssertion{
				{From: "", To: "a", StepCount: 1, Steps: []StepAssertion{{
					StepType:     sfvm.AnalysisStepTypeSearch,
					HasDesc:      true,
					DescContains: "Fuzzy Search",
				}}},
				{From: "a", To: "b", StepCount: 1, Steps: []StepAssertion{{
					StepType:         sfvm.AnalysisStepTypeConditionFilter,
					HasEvidenceTree:  true,
					EvidenceNodeType: sfvm.EvidenceTypeStringCondition,
					FilterType:       "string",
					HasPassedValues:  true,
					HasFailedValues:  true,
				}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			RunVarGraphTest(t, tt.VarGraphTestCase, tt.Edges)
		})
	}
}

// TestDataFlowFilterEvidence 测试 ?{} + 数据流分析的过滤证据
func TestDataFlowFilterEvidence(t *testing.T) {
	tests := []struct {
		VarGraphTestCase
		FilterEvidenceAssertion
	}{
		{
			VarGraphTestCase: VarGraphTestCase{
				Name: "filter with getUsers - verify intermediate values",
				Code: `
				a1 = "hello"
				b = a1 + "world"
				a2 = "foo"`,
				SFRule: "a* as $a;\n$a?{*<getUsers>} as $b;",
			},
			FilterEvidenceAssertion: FilterEvidenceAssertion{ExpectedPassedCount: 1, ExpectedFailedCount: 1, ExpectedTreeType: sfvm.EvidenceTypeFilterCondition, ExpectPassedHasFilterResult: true},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name: "filter with topdef - trace to source",
				Code: `
				source = "userInput"
				sink1 = source + "processed"
				sink2 = "constant" + "processed"`,
				SFRule: "source as $source;\nsink* as $sink;\n$sink?{* #-> & $source} as $tainted;",
			},
			FilterEvidenceAssertion: FilterEvidenceAssertion{ExpectedPassedCount: 1, ExpectedFailedCount: 1, ExpectedTreeType: sfvm.EvidenceTypeFilterCondition, ExpectPassedHasFilterResult: true},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name: "filter with bottomuse - has downstream",
				Code: `
				a = 1
				b = a + 2
				c = 3`,
				SFRule: `a as $source;
c as $other;
$source?{*-->} as $hasDownstream;`,
			},
			FilterEvidenceAssertion: FilterEvidenceAssertion{ExpectedPassedCount: 1, ExpectedFailedCount: 0, ExpectedTreeType: sfvm.EvidenceTypeFilterCondition, ExpectPassedHasFilterResult: true},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name: "filter with topdef config - until source",
				Code: `
				source = "userInput"
				middle = source + "transform"
				sink = middle + "execute"`,
				SFRule: "source as $source;\nsink as $sink;\n$sink?{* #{until: `* & $source`}->} as $reachable;",
			},
			FilterEvidenceAssertion: FilterEvidenceAssertion{ExpectedPassedCount: 1, ExpectedFailedCount: 0, ExpectedTreeType: sfvm.EvidenceTypeFilterCondition, ExpectPassedHasFilterResult: true},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name: "complex filter - dataflow AND string condition",
				Code: `
					a1 = "hello"
					b1 = a1 + "world"
					a2 = "foo"
					b2 = a2 + "bar"`,
				SFRule: "a* as $a;\n$a?{*<getUsers> && have:\"hello\"} as $filtered;",
			},
			FilterEvidenceAssertion: FilterEvidenceAssertion{
				ExpectedPassedCount: 2, ExpectedFailedCount: 0,
				ExpectedTreeType: sfvm.EvidenceTypeLogicGate, ExpectedLogicOp: sfvm.ConditionTypeAnd, ExpectedChildrenCount: 2,
				ExpectPassedHasFilterResult: true,
				CustomCheck: func(t *testing.T, tree *sfvm.EvidenceNode) {
					for i, child := range tree.Children {
						t.Logf("Child %d [%s]: Results count = %d", i, child.Type, len(child.Results))
					}
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			RunFilterEvidenceTest(t, tt.VarGraphTestCase, tt.FilterEvidenceAssertion)
		})
	}
}
