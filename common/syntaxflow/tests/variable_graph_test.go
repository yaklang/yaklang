package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestVariableGraphNodeAndEdge(t *testing.T) {
	type ExpectedEdge struct {
		From      string
		To        string
		StepTypes []sfvm.AnalysisStepType
	}

	tests := []struct {
		Name   string
		Code   string
		SFRule string
		Nodes  []string
		Edges  []ExpectedEdge
	}{
		{
			Name:   "Test Simple Exact Search",
			Code:   `source = "a"`,
			SFRule: "source as $source",
			Nodes:  []string{"source"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
			},
		},
		{
			Name: "Simple data flow",
			Code: `a = 1
				b = a + 2
				c = b * 3`,
			SFRule: `
				a as $source;
				$source-> as $sink
			`,
			Nodes: []string{"source", "sink"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "source",
					To:        "sink",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeDataFlow},
				},
			},
		},
		{
			Name: "Native call with getUsers",
			Code: `
				function target(x) {
					return x * 2
				}
				
				a = 10
				b = target(a)
			`,
			SFRule: `
				a as $var_a;
				$var_a<getUsers> as $var_b
			`,
			Nodes: []string{"var_a", "var_b"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "var_a",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "var_a",
					To:        "var_b",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform},
				},
			},
		},
		{
			Name: "Multi-step native call",
			Code: `
				a = 1;
				b = a + 2;
				c = b * 3;	
			`,
			SFRule: `
				a as $source;
				$source<getUsers><getUsers> as $sink;
			`,
			Nodes: []string{"source", "sink"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "source",
					To:        "sink",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform, sfvm.AnalysisStepTypeTransform},
				},
			},
		},
		{
			Name: "Multi-step data flow",
			Code: `
				a = 1;
				b = a + 2;
				c = b * 3;	
			`,
			SFRule: `
				a as $source;
				$source<getUsers>  as $sink1;
				$source -> as $sink2;
			`,
			Nodes: []string{"source", "sink1", "sink2"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "source",
					To:        "sink1",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform},
				},
				{
					From:      "source",
					To:        "sink2",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeDataFlow},
				},
			},
		},
		{
			Name: "Multi-step and chain ",
			Code: `
				a = 1;
				b = a + 2;
				c = b * 3;	
			`,
			SFRule: `
				a as $source;
				$source<getUsers> as $sink1;
				$sink1 -> as $sink2;
			`,
			Nodes: []string{"source", "sink1", "sink2"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "source",
					To:        "sink1",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform},
				},
				{
					From:      "sink1",
					To:        "sink2",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeDataFlow},
				},
			},
		},
		{
			Name: "simple condition filter",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{have: "hello"} as $sink1;
			`,
			Nodes: []string{"sink1"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink1",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter},
				},
			},
		},
		{
			Name: "simple condition filter with multiple conditions",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{have: "hello" && opcode:const} as $sink1;
			`,
			Nodes: []string{"sink1"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink1",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter},
				},
			},
		},
		{
			Name: "condition filter with filter statement",
			Code: `
			a1 = "hello"
			b = a1 + "aaa"
			a2="world"
			`,
			SFRule: `
			a* as $a;
			$a?{*<getUsers>} as $b;
			`,
			Nodes: []string{"a", "b"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "a",
					To:        "b",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter},
				},
			},
		},
		{
			Name: "recursive condition filter ",
			Code: `
			a1 = "hello"
			b = a1 + "aaa"
			a2="world"
			c = a2 + "aaa"
			`,
			SFRule: `
			a* as $a;
			$a?{opcode:const && *?{*<getUsers>} && have:"hello"} as $b;
			`,
			Nodes: []string{"a", "b"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "a",
					To:        "b",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter},
				},
			},
		},
		{
			Name: "chain condition filter ",
			Code: `
			a1 = "hello"
			a2= "world"
			`,
			SFRule: `
			a* as $a;
			$a?{any:"o"}?{opcode:const}?{have:"hello"} as $b;
			`,
			Nodes: []string{"a", "b"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "a",
					To:        "b",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter, sfvm.AnalysisStepTypeConditionFilter, sfvm.AnalysisStepTypeConditionFilter},
				},
			},
		},
		{
			Name: "condition filter with not logic gate",
			Code: `
			a1 = "hello"
			a2= "world"
			`,
			SFRule: `
			a* as $a;
			$a?{!have:"hello"} as $b;
			`,
			Nodes: []string{"a", "b"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "a",
					To:        "b",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter},
				},
			},
		},
		{
			Name: "condition filter with not logic gate 2",
			Code: `
			a1 = "hello"
			a2= "world"
			`,
			SFRule: `
			a* as $a;
			$a?{!have:"hello" && opcode:const} as $b;
			`,
			Nodes: []string{"a", "b"},
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch},
				},
				{
					From:      "a",
					To:        "b",
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			ssatest.Check(t, tt.Code, func(prog *ssaapi.Program) error {
				t.Logf("Test Case: %s", tt.Name)
				result, err := prog.SyntaxFlowWithError(tt.SFRule)
				require.NoError(t, err)

				graph := result.GetSFResult().GetVarGraph()
				require.NotNil(t, graph, "VarFlowGraph should not be nil")

				t.Logf("Graph:\n%s", graph.String())

				nodeIdToName := make(map[int]string)
				nodeIdToName[0] = ""
				for _, node := range graph.Nodes.Values() {
					nodeIdToName[node.NodeId] = node.VariableName
				}

				nodeNameSet := make(map[string]bool)
				for _, node := range graph.Nodes.Values() {
					nodeNameSet[node.VariableName] = true
				}
				for _, expectedNode := range tt.Nodes {
					require.True(t, nodeNameSet[expectedNode], "Node %s not found", expectedNode)
				}

				for _, expectedEdge := range tt.Edges {
					found := false
					for _, edge := range graph.Edges {
						fromName := nodeIdToName[edge.FromNodeId]
						toName := nodeIdToName[edge.ToNodeId]

						if fromName == expectedEdge.From && toName == expectedEdge.To {
							found = true

							if len(expectedEdge.StepTypes) > 0 {
								stepTypeSet := make(map[sfvm.AnalysisStepType]bool)
								for _, stepId := range edge.Steps {
									step, ok := graph.Steps.Get(stepId)
									if ok {
										stepTypeSet[step.StepType] = true
									}
								}
								for _, expectedType := range expectedEdge.StepTypes {
									require.True(t, stepTypeSet[expectedType],
										"Edge %s -> %s should have step type %s, got: %v",
										expectedEdge.From, expectedEdge.To, expectedType, stepTypeSet)
								}
							}
							break
						}
					}
					require.True(t, found, "Edge from %q to %q not found", expectedEdge.From, expectedEdge.To)
				}
				return nil
			})
		})
	}
}

// TestVariableGraphEvidenceAttach 测试证据挂载功能
func TestVariableGraphEvidenceAttach(t *testing.T) {
	type EvidenceAssertion struct {
		// 基本属性
		StepType sfvm.AnalysisStepType
		// 证据树检查（用于过滤语句）
		HasEvidenceTree bool
		// 证据树类型检查
		EvidenceNodeType sfvm.EvidenceNodeType
		// 逻辑操作符检查
		LogicOp sfvm.EvidenceNodeCondition
		// 过滤器类型检查
		FilterType string
		// 子节点数量
		ChildrenCount int
		// 是否有 Passed/Failed 值
		HasPassedValues bool
		HasFailedValues bool
		// 搜索/数据流证据检查
		HasDesc   bool
		HasDescZh bool
		HasValues bool
		// Label 包含的关键字
		DescContains string
		// 搜索模式检查
		HasSearchMode   bool
		SearchMatchMode int
	}

	type ExpectedEdge struct {
		From            string
		To              string
		StepCount       int                 // 总步骤数量
		EvidenceAsserts []EvidenceAssertion // 对每个步骤的证据断言
	}

	tests := []struct {
		Name   string
		Code   string
		SFRule string
		Edges  []ExpectedEdge
	}{
		// ==================== 搜索语句证据测试 ====================
		{
			Name:   "exact search evidence - name match",
			Code:   `source = "hello"`,
			SFRule: `source as $source;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:        sfvm.AnalysisStepTypeSearch,
							HasDesc:         true,
							HasDescZh:       true,
							HasValues:       true,
							DescContains:    "Exact Search",
							HasSearchMode:   true,
							SearchMatchMode: sfvm.NameMatch | sfvm.KeyMatch,
						},
					},
				},
			},
		},
		{
			Name: "exact search evidence - key match (member access)",
			Code: `
				a = {"source": "hello"}
			`,
			SFRule: `.source as $source;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:        sfvm.AnalysisStepTypeSearch,
							HasDesc:         true,
							HasDescZh:       true,
							HasValues:       true,
							DescContains:    "Exact Search",
							HasSearchMode:   true,
							SearchMatchMode: sfvm.KeyMatch, // 成员访问只用 KeyMatch
						},
					},
				},
			},
		},
		{
			Name:   "fuzzy search evidence",
			Code:   `a1 = "hello"; a2 = "world"`,
			SFRule: `a* as $a;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							HasDescZh:    true,
							HasValues:    true,
							DescContains: "Fuzzy Search",
						},
					},
				},
			},
		},
		{
			Name:   "regexp search evidence",
			Code:   `abc = "hello"; xyz = "world"`,
			SFRule: `/a.*/ as $a;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							HasDescZh:    true,
							HasValues:    true,
							DescContains: "Regexp Search",
						},
					},
				},
			},
		},
		// ==================== 数据流语句证据测试 ====================
		{
			Name:   "data flow get users evidence",
			Code:   `a = 1; b = a + 2; c = b * 3`,
			SFRule: `a as $source; $source-> as $sink;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "source",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Exact Search",
						},
					},
				},
				{
					From:      "source",
					To:        "sink",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeDataFlow,
							HasDesc:      true,
							HasDescZh:    true,
							HasValues:    true,
							DescContains: "Get Users",
						},
					},
				},
			},
		},
		{
			Name:   "data flow top defs evidence",
			Code:   `a = 1; b = a + 2; c = b * 3`,
			SFRule: `c as $sink; $sink #-> as $source;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Exact Search",
						},
					},
				},
				{
					From:      "sink",
					To:        "source",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeDataFlow,
							HasDesc:      true,
							HasDescZh:    true,
							HasValues:    true,
							DescContains: "Get TopDefs",
						},
					},
				},
			},
		},
		{
			Name: "native call getUsers evidence",
			Code: `
				function target(x) { return x * 2 }
				a = 10
				b = target(a)
			`,
			SFRule: `a as $var_a; $var_a<getUsers> as $var_b;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "var_a",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Exact Search",
						},
					},
				},
				{
					From:      "var_a",
					To:        "var_b",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeTransform,
							HasDesc:      true,
							HasDescZh:    true,
							HasValues:    true,
							DescContains: "Native Call",
						},
					},
				},
			},
		},
		{
			Name:   "native call string transform evidence",
			Code:   `a = "hello"`,
			SFRule: `a as $a; $a<string> as $b;`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Exact Search",
						},
					},
				},
				{
					From:      "a",
					To:        "b",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeTransform,
							HasDesc:      true,
							HasDescZh:    true,
							HasValues:    true,
							DescContains: "Native Call",
						},
					},
				},
			},
		},
		{
			Name: "simple string filter evidence",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{have: "hello"} as $sink;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 2, // Search + Filter
					EvidenceAsserts: []EvidenceAssertion{
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
							HasPassedValues:  true,
							HasFailedValues:  true,
						},
					},
				},
			},
		},
		{
			Name: "opcode filter evidence",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{opcode: const} as $sink;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 2,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Fuzzy Search",
						},
						{
							StepType:         sfvm.AnalysisStepTypeConditionFilter,
							HasEvidenceTree:  true,
							EvidenceNodeType: sfvm.EvidenceTypeOpcodeCondition,
							FilterType:       "opcode",
							HasPassedValues:  true,
							HasFailedValues:  false, // 全部通过
						},
					},
				},
			},
		},
		{
			Name: "logic AND evidence",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{have: "hello" && opcode: const} as $sink;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 2,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Fuzzy Search",
						},
						{
							StepType:         sfvm.AnalysisStepTypeConditionFilter,
							HasEvidenceTree:  true,
							EvidenceNodeType: sfvm.EvidenceTypeLogicGate,
							LogicOp:          sfvm.ConditionTypeAnd,
							ChildrenCount:    2,
						},
					},
				},
			},
		},
		{
			Name: "logic OR evidence",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{have: "hello" || have: "world"} as $sink;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 2,
					EvidenceAsserts: []EvidenceAssertion{
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
					},
				},
			},
		},
		{
			Name: "logic NOT evidence",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{!have: "hello"} as $sink;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 2,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Fuzzy Search",
						},
						{
							StepType:         sfvm.AnalysisStepTypeConditionFilter,
							HasEvidenceTree:  true,
							EvidenceNodeType: sfvm.EvidenceTypeLogicGate,
							LogicOp:          sfvm.ConditionTypeNot,
							ChildrenCount:    1,
						},
					},
				},
			},
		},
		{
			Name: "chain filter evidence - multiple steps",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* ?{have: "o"}?{opcode: const} as $sink;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 3, // Search + Filter + Filter
					EvidenceAsserts: []EvidenceAssertion{
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
						{
							StepType:         sfvm.AnalysisStepTypeConditionFilter,
							HasEvidenceTree:  true,
							EvidenceNodeType: sfvm.EvidenceTypeOpcodeCondition,
							FilterType:       "opcode",
						},
					},
				},
			},
		},
		{
			Name: "complex logic: (A && B) || C",
			Code: `
			a1 = "hello"
			a2 = "world"
			a3 = "foo"
			`,
			SFRule: `
			a* ?{(have: "hello" && opcode: const) || have: "world"} as $sink;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "sink",
					StepCount: 2,
					EvidenceAsserts: []EvidenceAssertion{
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
							ChildrenCount:    2, // AND 节点 和 have:"world" 节点
						},
					},
				},
			},
		},
		{
			Name: "filter with source variable",
			Code: `
			a1 = "hello"
			a2 = "world"
			`,
			SFRule: `
			a* as $a;
			$a?{have: "hello"} as $b;
			`,
			Edges: []ExpectedEdge{
				{
					From:      "",
					To:        "a",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:     sfvm.AnalysisStepTypeSearch,
							HasDesc:      true,
							DescContains: "Fuzzy Search",
						},
					},
				},
				{
					From:      "a",
					To:        "b",
					StepCount: 1,
					EvidenceAsserts: []EvidenceAssertion{
						{
							StepType:         sfvm.AnalysisStepTypeConditionFilter,
							HasEvidenceTree:  true,
							EvidenceNodeType: sfvm.EvidenceTypeStringCondition,
							FilterType:       "string",
							HasPassedValues:  true,
							HasFailedValues:  true,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			ssatest.Check(t, tt.Code, func(prog *ssaapi.Program) error {
				t.Logf("Test Case: %s", tt.Name)
				result, err := prog.SyntaxFlowWithError(tt.SFRule)
				require.NoError(t, err)

				graph := result.GetSFResult().GetVarGraph()
				require.NotNil(t, graph, "VarFlowGraph should not be nil")

				t.Logf("Graph:\n%s", graph.String())

				// 构建节点ID到名称的映射
				nodeIdToName := make(map[int]string)
				nodeIdToName[0] = ""
				for _, node := range graph.Nodes.Values() {
					nodeIdToName[node.NodeId] = node.VariableName
				}

				// 验证每条边的证据
				for _, expectedEdge := range tt.Edges {
					found := false
					for _, edge := range graph.Edges {
						fromName := nodeIdToName[edge.FromNodeId]
						toName := nodeIdToName[edge.ToNodeId]

						if fromName == expectedEdge.From && toName == expectedEdge.To {
							found = true

							// 收集所有步骤（带有证据的）
							var allSteps []*sfvm.AnalysisStep
							for _, stepId := range edge.Steps {
								step, ok := graph.Steps.Get(stepId)
								if ok && step.EvidenceAttach != nil {
									allSteps = append(allSteps, step)
								}
							}

							// 验证步骤数量
							require.Equal(t, expectedEdge.StepCount, len(allSteps),
								"Edge %s -> %s: expected %d steps with evidence, got %d",
								expectedEdge.From, expectedEdge.To, expectedEdge.StepCount, len(allSteps))

							// 验证每个步骤的证据
							for i, assertion := range expectedEdge.EvidenceAsserts {
								if i >= len(allSteps) {
									break
								}
								step := allSteps[i]
								evidence := step.EvidenceAttach

								// 验证步骤类型
								require.Equal(t, assertion.StepType, step.StepType,
									"Step %d: expected type %s, got %s", i, assertion.StepType, step.StepType)

								if assertion.HasDesc {
									require.NotEmpty(t, evidence.GetDescription(),
										"Step %d: expected desc", i)
								}
								if assertion.HasDescZh {
									require.NotEmpty(t, evidence.GetDescriptionZh(),
										"Step %d: expected DescriptionZh", i)
								}
								if assertion.HasValues {
									require.NotNil(t, evidence.Values,
										"Step %d: expected Values", i)
								}
								if assertion.DescContains != "" {
									require.Contains(t, evidence.GetDescription(), assertion.DescContains,
										"Step %d: Label should contain %q, got %q", i, assertion.DescContains, evidence.GetDescription())
								}

								// 验证搜索模式（搜索语句的证据）
								if assertion.HasSearchMode {
									require.NotNil(t, evidence.SearchMode,
										"Step %d: expected SearchMode", i)
									if assertion.SearchMatchMode != 0 {
										require.Equal(t, assertion.SearchMatchMode, evidence.SearchMode.MatchMode,
											"Step %d: expected match mode %d (%s), got %d (%s)",
											i, assertion.SearchMatchMode, sfvm.MatchModeString(assertion.SearchMatchMode),
											evidence.SearchMode.MatchMode, evidence.SearchMode.MatchModeStr)
									}
								}

								// 验证证据树存在（过滤语句的证据）
								tree := evidence.EvidenceTree
								if assertion.HasEvidenceTree {
									require.NotNil(t, tree, "Step %d: expected evidence tree", i)

									// 验证证据节点类型
									if assertion.EvidenceNodeType != "" {
										require.Equal(t, assertion.EvidenceNodeType, tree.Type,
											"Step %d: expected node type %s, got %s", i, assertion.EvidenceNodeType, tree.Type)
									}

									// 验证逻辑操作符
									if assertion.LogicOp != "" {
										require.Equal(t, assertion.LogicOp, tree.LogicOp,
											"Step %d: expected logic op %s, got %s", i, assertion.LogicOp, tree.LogicOp)
									}

									// 验证过滤器类型
									if assertion.FilterType != "" && tree.Filter != nil {
										require.Equal(t, assertion.FilterType, tree.Filter.FilterType,
											"Step %d: expected filter type %s, got %s", i, assertion.FilterType, tree.Filter.FilterType)
									}

									// 验证子节点数量
									if assertion.ChildrenCount > 0 {
										require.Equal(t, assertion.ChildrenCount, len(tree.Children),
											"Step %d: expected %d children, got %d", i, assertion.ChildrenCount, len(tree.Children))
									}

									// 验证 Passed/Failed 值
									if assertion.HasPassedValues {
										require.NotNil(t, tree.Passed,
											"Step %d: expected Passed values", i)
									}
									if assertion.HasFailedValues {
										require.NotNil(t, tree.Failed,
											"Step %d: expected Failed values", i)
									}
								}
							}
							break
						}
					}
					require.True(t, found, "Edge from %q to %q not found", expectedEdge.From, expectedEdge.To)
				}
				return nil
			})
		})
	}
}

// TestVariableGraphEvidenceTreeStructure 测试证据树的结构完整性
func TestVariableGraphEvidenceTreeStructure(t *testing.T) {
	t.Run("nested AND structure", func(t *testing.T) {
		code := `
		a1 = "hello"
		a2 = "world"
		`
		sfRule := `a* ?{have: "hello" && opcode: const} as $sink;`

		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			result, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)

			graph := result.GetSFResult().GetVarGraph()
			require.NotNil(t, graph)

			t.Logf("Graph:\n%s", graph.String())

			// 找到过滤步骤
			var filterStep *sfvm.AnalysisStep
			graph.Steps.ForEach(func(key int, step *sfvm.AnalysisStep) bool {
				if step.StepType == sfvm.AnalysisStepTypeConditionFilter && step.EvidenceAttach != nil {
					filterStep = step
					return false
				}
				return true
			})

			require.NotNil(t, filterStep, "Filter step not found")
			require.NotNil(t, filterStep.EvidenceAttach.EvidenceTree, "Evidence tree not found")

			tree := filterStep.EvidenceAttach.EvidenceTree

			// 验证根节点是 AND 逻辑门
			require.Equal(t, sfvm.EvidenceTypeLogicGate, tree.Type)
			require.Equal(t, sfvm.ConditionTypeAnd, tree.LogicOp)
			require.Equal(t, 2, len(tree.Children), "AND node should have 2 children")

			// 验证子节点
			var stringChild, opcodeChild *sfvm.EvidenceNode
			for _, child := range tree.Children {
				switch child.Type {
				case sfvm.EvidenceTypeStringCondition:
					stringChild = child
				case sfvm.EvidenceTypeOpcodeCondition:
					opcodeChild = child
				}
			}

			require.NotNil(t, stringChild, "String condition child not found")
			require.NotNil(t, opcodeChild, "Opcode condition child not found")

			// 验证字符串条件
			require.NotNil(t, stringChild.Filter)
			require.Equal(t, "string", stringChild.Filter.FilterType)
			require.Equal(t, "have", stringChild.Filter.MatchMode)

			// 验证 opcode 条件
			require.NotNil(t, opcodeChild.Filter)
			require.Equal(t, "opcode", opcodeChild.Filter.FilterType)

			return nil
		})
	})

	t.Run("nested OR structure", func(t *testing.T) {
		code := `
		a1 = "hello"
		a2 = "world"
		`
		sfRule := `a* ?{have: "hello" || have: "world"} as $sink;`

		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			result, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)

			graph := result.GetSFResult().GetVarGraph()
			require.NotNil(t, graph)

			t.Logf("Graph:\n%s", graph.String())

			// 找到过滤步骤
			var filterStep *sfvm.AnalysisStep
			graph.Steps.ForEach(func(key int, step *sfvm.AnalysisStep) bool {
				if step.StepType == sfvm.AnalysisStepTypeConditionFilter && step.EvidenceAttach != nil {
					filterStep = step
					return false
				}
				return true
			})

			require.NotNil(t, filterStep, "Filter step not found")
			tree := filterStep.EvidenceAttach.EvidenceTree

			// 验证根节点是 OR 逻辑门
			require.Equal(t, sfvm.EvidenceTypeLogicGate, tree.Type)
			require.Equal(t, sfvm.ConditionTypeOr, tree.LogicOp)
			require.Equal(t, 2, len(tree.Children), "OR node should have 2 children")

			// 验证两个子节点都是字符串条件
			for _, child := range tree.Children {
				require.Equal(t, sfvm.EvidenceTypeStringCondition, child.Type)
				require.NotNil(t, child.Filter)
				require.Equal(t, "string", child.Filter.FilterType)
			}

			return nil
		})
	})

	t.Run("NOT structure", func(t *testing.T) {
		code := `
		a1 = "hello"
		a2 = "world"
		`
		sfRule := `a* ?{!have: "hello"} as $sink;`

		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			result, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)

			graph := result.GetSFResult().GetVarGraph()
			require.NotNil(t, graph)

			t.Logf("Graph:\n%s", graph.String())

			// 找到过滤步骤
			var filterStep *sfvm.AnalysisStep
			graph.Steps.ForEach(func(key int, step *sfvm.AnalysisStep) bool {
				if step.StepType == sfvm.AnalysisStepTypeConditionFilter && step.EvidenceAttach != nil {
					filterStep = step
					return false
				}
				return true
			})

			require.NotNil(t, filterStep, "Filter step not found")
			tree := filterStep.EvidenceAttach.EvidenceTree

			// 验证根节点是 NOT 逻辑门
			require.Equal(t, sfvm.EvidenceTypeLogicGate, tree.Type)
			require.Equal(t, sfvm.ConditionTypeNot, tree.LogicOp)
			require.Equal(t, 1, len(tree.Children), "NOT node should have 1 child")

			// 验证子节点是字符串条件
			child := tree.Children[0]
			require.Equal(t, sfvm.EvidenceTypeStringCondition, child.Type)

			return nil
		})
	})
}
