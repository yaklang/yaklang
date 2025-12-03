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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeGet},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeGet, sfvm.AnalysisStepTypeGet},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeGet},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeGet},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeFilter},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeFilter},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeFilter},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeFilter},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeFilter, sfvm.AnalysisStepTypeFilter, sfvm.AnalysisStepTypeFilter},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeFilter},
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
					StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeFilter},
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
