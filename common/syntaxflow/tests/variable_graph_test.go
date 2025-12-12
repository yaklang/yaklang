package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// ==================== 公共类型定义 ====================

// VarGraphTestCase 通用的变量图测试用例
type VarGraphTestCase struct {
	Name   string
	Code   string
	SFRule string
}

// EdgeAssertion 边的断言配置
type EdgeAssertion struct {
	From      string // 源节点名称，空字符串表示起始节点
	To        string // 目标节点名称
	StepCount int    // 期望的步骤数量
	StepTypes []sfvm.AnalysisStepType
	Steps     []StepAssertion // 每个步骤的断言
}

// StepAssertion 步骤的断言配置
type StepAssertion struct {
	// 基本属性
	StepType sfvm.AnalysisStepType

	// 描述检查
	HasDesc      bool
	HasDescZh    bool
	DescContains string

	// 值检查
	HasValues bool

	// 搜索模式检查
	HasSearchMode   bool
	SearchMatchMode int

	// 数据流模式检查
	HasDataFlow   bool
	DataFlowCheck func(t *testing.T, mode *sfvm.DataFlowMode)

	// 证据树检查
	HasEvidenceTree  bool
	EvidenceNodeType sfvm.EvidenceNodeType
	LogicOp          sfvm.EvidenceNodeCondition
	FilterType       string
	ChildrenCount    int
	HasPassedValues  bool
	HasFailedValues  bool

	// 自定义验证
	CustomCheck func(t *testing.T, step *sfvm.AnalysisStep)
}

// FilterEvidenceAssertion 过滤证据的断言配置
type FilterEvidenceAssertion struct {
	ExpectedPassedCount         int
	ExpectedFailedCount         int
	ExpectedTreeType            sfvm.EvidenceNodeType
	ExpectedLogicOp             sfvm.EvidenceNodeCondition
	ExpectedChildrenCount       int
	ExpectPassedHasFilterResult bool
	CustomCheck                 func(t *testing.T, tree *sfvm.EvidenceNode)
}

// ==================== 公共辅助函数 ====================

// buildNodeIdToNameMap 构建节点 ID 到名称的映射
func buildNodeIdToNameMap(graph *sfvm.VarFlowGraph) map[int]string {
	nodeIdToName := make(map[int]string)
	nodeIdToName[0] = ""
	for _, node := range graph.Nodes.Values() {
		nodeIdToName[node.NodeId] = node.VariableName
	}
	return nodeIdToName
}

// findFilterStep 在 VarFlowGraph 中查找第一个过滤步骤
func findFilterStep(graph *sfvm.VarFlowGraph) *sfvm.AnalysisStep {
	var filterStep *sfvm.AnalysisStep
	graph.Steps.ForEach(func(key int, step *sfvm.AnalysisStep) bool {
		if step.StepType == sfvm.AnalysisStepTypeConditionFilter && step.EvidenceAttach != nil {
			filterStep = step
			return false
		}
		return true
	})
	return filterStep
}

// countFilterResults 统计 Results 中通过和未通过的数量
func countFilterResults(results []*sfvm.FilterResult) (passed, failed int) {
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}

// getStepsWithEvidence 获取边上所有带有证据的步骤
func getStepsWithEvidence(graph *sfvm.VarFlowGraph, edge *sfvm.VarFlowEdge) []*sfvm.AnalysisStep {
	var steps []*sfvm.AnalysisStep
	for _, stepId := range edge.Steps {
		step, ok := graph.Steps.Get(stepId)
		if ok && step.EvidenceAttach != nil {
			steps = append(steps, step)
		}
	}
	return steps
}

// ==================== 通用测试运行器 ====================

// RunVarGraphTest 运行变量图测试
func RunVarGraphTest(t *testing.T, tc VarGraphTestCase, edges []EdgeAssertion) {
	ssatest.Check(t, tc.Code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(tc.SFRule)
		require.NoError(t, err)

		graph := result.GetSFResult().GetVarGraph()
		require.NotNil(t, graph, "VarFlowGraph should not be nil")

		t.Logf("Graph:\n%s", graph.String())

		nodeIdToName := buildNodeIdToNameMap(graph)

		for _, expectedEdge := range edges {
			assertEdge(t, graph, nodeIdToName, expectedEdge)
		}
		return nil
	})
}

// assertEdge 验证单条边
func assertEdge(t *testing.T, graph *sfvm.VarFlowGraph, nodeIdToName map[int]string, expected EdgeAssertion) {
	found := false
	for _, edge := range graph.Edges {
		fromName := nodeIdToName[edge.FromNodeId]
		toName := nodeIdToName[edge.ToNodeId]

		if fromName == expected.From && toName == expected.To {
			found = true

			// 验证步骤类型（如果指定）
			if len(expected.StepTypes) > 0 {
				stepTypeSet := make(map[sfvm.AnalysisStepType]bool)
				for _, stepId := range edge.Steps {
					step, ok := graph.Steps.Get(stepId)
					if ok {
						stepTypeSet[step.StepType] = true
					}
				}
				for _, expectedType := range expected.StepTypes {
					require.True(t, stepTypeSet[expectedType],
						"Edge %s -> %s should have step type %s", expected.From, expected.To, expectedType)
				}
			}

			// 获取带有证据的步骤
			allSteps := getStepsWithEvidence(graph, edge)

			// 验证步骤数量
			if expected.StepCount > 0 {
				require.Equal(t, expected.StepCount, len(allSteps),
					"Edge %s -> %s: expected %d steps with evidence, got %d",
					expected.From, expected.To, expected.StepCount, len(allSteps))
			}

			// 验证每个步骤
			for i, stepAssert := range expected.Steps {
				if i >= len(allSteps) {
					break
				}
				assertStep(t, i, allSteps[i], stepAssert)
			}
			break
		}
	}
	require.True(t, found, "Edge from %q to %q not found", expected.From, expected.To)
}

// assertStep 验证单个步骤
func assertStep(t *testing.T, idx int, step *sfvm.AnalysisStep, assert StepAssertion) {
	evidence := step.EvidenceAttach

	// 验证步骤类型
	if assert.StepType != 0 {
		require.Equal(t, assert.StepType, step.StepType,
			"Step %d: expected type %s, got %s", idx, assert.StepType, step.StepType)
	}

	// 验证描述
	if assert.HasDesc {
		require.NotEmpty(t, evidence.GetDescription(), "Step %d: expected desc", idx)
	}
	if assert.HasDescZh {
		require.NotEmpty(t, evidence.GetDescriptionZh(), "Step %d: expected DescriptionZh", idx)
	}
	if assert.DescContains != "" {
		require.Contains(t, evidence.GetDescription(), assert.DescContains,
			"Step %d: desc should contain %q, got %q", idx, assert.DescContains, evidence.GetDescription())
	}

	// 验证值
	if assert.HasValues {
		require.NotNil(t, evidence.Values, "Step %d: expected Values", idx)
	}

	// 验证搜索模式
	if assert.HasSearchMode {
		require.NotNil(t, evidence.SearchMode, "Step %d: expected SearchMode", idx)
		if assert.SearchMatchMode != 0 {
			require.Equal(t, assert.SearchMatchMode, evidence.SearchMode.MatchMode,
				"Step %d: expected match mode %d (%s), got %d (%s)",
				idx, assert.SearchMatchMode, sfvm.MatchModeString(assert.SearchMatchMode),
				evidence.SearchMode.MatchMode, evidence.SearchMode.MatchModeStr)
		}
	}

	// 验证数据流模式
	if assert.HasDataFlow {
		require.NotNil(t, evidence.DataFlowMode, "Step %d: expected DataFlowMode", idx)
		if assert.DataFlowCheck != nil {
			assert.DataFlowCheck(t, evidence.DataFlowMode)
		}
	}

	// 验证证据树
	if assert.HasEvidenceTree {
		tree := evidence.EvidenceTree
		require.NotNil(t, tree, "Step %d: expected evidence tree", idx)

		if assert.EvidenceNodeType != "" {
			require.Equal(t, assert.EvidenceNodeType, tree.Type,
				"Step %d: expected node type %s, got %s", idx, assert.EvidenceNodeType, tree.Type)
		}
		if assert.LogicOp != "" {
			require.Equal(t, assert.LogicOp, tree.LogicOp,
				"Step %d: expected logic op %s, got %s", idx, assert.LogicOp, tree.LogicOp)
		}
		if assert.FilterType != "" && tree.CompareEvidence != nil {
			require.Equal(t, assert.FilterType, tree.CompareEvidence.FilterType,
				"Step %d: expected filter type %s, got %s", idx, assert.FilterType, tree.CompareEvidence.FilterType)
		}
		if assert.ChildrenCount > 0 {
			require.Equal(t, assert.ChildrenCount, len(tree.Children),
				"Step %d: expected %d children, got %d", idx, assert.ChildrenCount, len(tree.Children))
		}

		// 验证 Results
		if assert.HasPassedValues || assert.HasFailedValues {
			// 优先从当前节点获取 Results，如果没有则从子节点收集
			results := tree.Results
			if len(results) == 0 {
				results = collectResults(tree)
			}
			require.NotEmpty(t, results, "Step %d: expected Results", idx)
			if assert.HasPassedValues {
				hasPassed := false
				for _, r := range results {
					if r.Passed {
						hasPassed = true
						break
					}
				}
				require.True(t, hasPassed, "Step %d: expected passed values", idx)
			}
			if assert.HasFailedValues {
				hasFailed := false
				for _, r := range results {
					if !r.Passed {
						hasFailed = true
						break
					}
				}
				require.True(t, hasFailed, "Step %d: expected failed values", idx)
			}
		}
	}

	// 自定义验证
	if assert.CustomCheck != nil {
		assert.CustomCheck(t, step)
	}
}

// findFilterConditionNodes 从证据树中找到所有 FilterCondition 类型的节点
func findFilterConditionNodes(node *sfvm.EvidenceNode) []*sfvm.EvidenceNode {
	if node == nil {
		return nil
	}
	var nodes []*sfvm.EvidenceNode
	if node.Type == sfvm.EvidenceTypeFilterCondition {
		nodes = append(nodes, node)
	}
	for _, child := range node.Children {
		nodes = append(nodes, findFilterConditionNodes(child)...)
	}
	return nodes
}

// collectResults 从证据树中收集所有 FilterCondition 节点的 Results
func collectResults(node *sfvm.EvidenceNode) []*sfvm.FilterResult {
	if node == nil {
		return nil
	}
	// 如果是 FilterCondition 节点，直接返回其 Results
	if node.Type == sfvm.EvidenceTypeFilterCondition {
		return node.Results
	}
	// 如果是 LogicGate 节点，递归收集子节点的 Results
	var results []*sfvm.FilterResult
	for _, child := range node.Children {
		results = append(results, collectResults(child)...)
	}
	return results
}

// RunFilterEvidenceTest 运行过滤证据测试
func RunFilterEvidenceTest(t *testing.T, tc VarGraphTestCase, assert FilterEvidenceAssertion) {
	ssatest.Check(t, tc.Code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(tc.SFRule)
		require.NoError(t, err)

		graph := result.GetSFResult().GetVarGraph()
		require.NotNil(t, graph)
		t.Logf("Graph:\n%s", graph.String())

		filterStep := findFilterStep(graph)
		require.NotNil(t, filterStep, "Filter step not found")

		tree := filterStep.EvidenceAttach.EvidenceTree
		require.NotNil(t, tree, "Evidence tree not found")

		// 验证证据树类型
		require.Equal(t, assert.ExpectedTreeType, tree.Type,
			"Expected tree type %s, got %s", assert.ExpectedTreeType, tree.Type)

		// 验证逻辑操作符
		if assert.ExpectedTreeType == sfvm.EvidenceTypeLogicGate {
			require.Equal(t, assert.ExpectedLogicOp, tree.LogicOp,
				"Expected logic op %s, got %s", assert.ExpectedLogicOp, tree.LogicOp)
			if assert.ExpectedChildrenCount > 0 {
				require.Equal(t, assert.ExpectedChildrenCount, len(tree.Children),
					"Expected %d children, got %d", assert.ExpectedChildrenCount, len(tree.Children))
			}
		}

		// 验证所有 FilterCondition 节点都有 Results
		filterConditionNodes := findFilterConditionNodes(tree)
		require.NotEmpty(t, filterConditionNodes, "FilterCondition nodes should exist")
		for i, fcNode := range filterConditionNodes {
			require.NotEmpty(t, fcNode.Results,
				"FilterCondition node %d should have Results", i)
			t.Logf("FilterCondition node %d has %d results", i, len(fcNode.Results))
		}

		// 收集 Results：从 FilterCondition 节点收集
		results := collectResults(tree)
		require.NotEmpty(t, results, "Results should not be empty")

		passedCount, failedCount := countFilterResults(results)
		require.Equal(t, assert.ExpectedPassedCount, passedCount,
			"Expected %d passed, got %d", assert.ExpectedPassedCount, passedCount)
		require.Equal(t, assert.ExpectedFailedCount, failedCount,
			"Expected %d failed, got %d", assert.ExpectedFailedCount, failedCount)

		// 验证 IntermValue (中间值)
		for _, r := range results {
			if r.Passed {
				t.Logf("Passed: %v -> IntermValue: %v", r.Value, r.IntermValue)
				if assert.ExpectPassedHasFilterResult {
					require.NotNil(t, r.IntermValue, "Passed value should have IntermValue")
					require.False(t, r.IntermValue.IsEmpty(), "IntermValue should not be empty")
				}
			} else {
				t.Logf("Failed: %v", r.Value)
			}
		}

		// 自定义验证
		if assert.CustomCheck != nil {
			assert.CustomCheck(t, tree)
		}

		return nil
	})
}

func TestVariableGraphNodeAndEdge(t *testing.T) {
	tests := []struct {
		VarGraphTestCase
		Nodes []string
		Edges []EdgeAssertion
	}{
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "Test Simple Exact Search",
				Code:   `source = "a"`,
				SFRule: "source as $source",
			},
			Nodes: []string{"source"},
			Edges: []EdgeAssertion{
				{From: "", To: "source", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name: "Simple data flow",
				Code: `a = 1
				b = a + 2
				c = b * 3`,
				SFRule: `
				a as $source;
				$source-> as $sink
			`,
			},
			Nodes: []string{"source", "sink"},
			Edges: []EdgeAssertion{
				{From: "", To: "source", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "source", To: "sink", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeDataFlow}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
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
			},
			Nodes: []string{"var_a", "var_b"},
			Edges: []EdgeAssertion{
				{From: "", To: "var_a", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "var_a", To: "var_b", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
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
			},
			Nodes: []string{"source", "sink"},
			Edges: []EdgeAssertion{
				{From: "", To: "source", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "source", To: "sink", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform, sfvm.AnalysisStepTypeTransform}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
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
			},
			Nodes: []string{"source", "sink1", "sink2"},
			Edges: []EdgeAssertion{
				{From: "", To: "source", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "source", To: "sink1", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform}},
				{From: "source", To: "sink2", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeDataFlow}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name: "Multi-step and chain",
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
			},
			Nodes: []string{"source", "sink1", "sink2"},
			Edges: []EdgeAssertion{
				{From: "", To: "source", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "source", To: "sink1", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeTransform}},
				{From: "sink1", To: "sink2", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeDataFlow}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "simple condition filter",
				Code:   "a1 = \"hello\"\na2 = \"world\"",
				SFRule: `a* ?{have: "hello"} as $sink1;`,
			},
			Nodes: []string{"sink1"},
			Edges: []EdgeAssertion{
				{From: "", To: "sink1", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "simple condition filter with multiple conditions",
				Code:   "a1 = \"hello\"\na2 = \"world\"",
				SFRule: `a* ?{have: "hello" && opcode:const} as $sink1;`,
			},
			Nodes: []string{"sink1"},
			Edges: []EdgeAssertion{
				{From: "", To: "sink1", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "condition filter with filter statement",
				Code:   "a1 = \"hello\"\nb = a1 + \"aaa\"\na2=\"world\"",
				SFRule: "a* as $a;\n$a?{*<getUsers>} as $b;",
			},
			Nodes: []string{"a", "b"},
			Edges: []EdgeAssertion{
				{From: "", To: "a", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "a", To: "b", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "recursive condition filter",
				Code:   "a1 = \"hello\"\nb = a1 + \"aaa\"\na2=\"world\"\nc = a2 + \"aaa\"",
				SFRule: "a* as $a;\n$a?{opcode:const && *?{*<getUsers>} && have:\"hello\"} as $b;",
			},
			Nodes: []string{"a", "b"},
			Edges: []EdgeAssertion{
				{From: "", To: "a", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "a", To: "b", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "chain condition filter",
				Code:   "a1 = \"hello\"\na2= \"world\"",
				SFRule: "a* as $a;\n$a?{any:\"o\"}?{opcode:const}?{have:\"hello\"} as $b;",
			},
			Nodes: []string{"a", "b"},
			Edges: []EdgeAssertion{
				{From: "", To: "a", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "a", To: "b", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter, sfvm.AnalysisStepTypeConditionFilter, sfvm.AnalysisStepTypeConditionFilter}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "condition filter with not logic gate",
				Code:   "a1 = \"hello\"\na2= \"world\"",
				SFRule: "a* as $a;\n$a?{!have:\"hello\"} as $b;",
			},
			Nodes: []string{"a", "b"},
			Edges: []EdgeAssertion{
				{From: "", To: "a", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "a", To: "b", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter}},
			},
		},
		{
			VarGraphTestCase: VarGraphTestCase{
				Name:   "condition filter with not logic gate 2",
				Code:   "a1 = \"hello\"\na2= \"world\"",
				SFRule: "a* as $a;\n$a?{!have:\"hello\" && opcode:const} as $b;",
			},
			Nodes: []string{"a", "b"},
			Edges: []EdgeAssertion{
				{From: "", To: "a", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeSearch}},
				{From: "a", To: "b", StepTypes: []sfvm.AnalysisStepType{sfvm.AnalysisStepTypeConditionFilter}},
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

				// 验证节点
				nodeNameSet := make(map[string]bool)
				for _, node := range graph.Nodes.Values() {
					nodeNameSet[node.VariableName] = true
				}
				for _, expectedNode := range tt.Nodes {
					require.True(t, nodeNameSet[expectedNode], "Node %s not found", expectedNode)
				}

				// 验证边
				nodeIdToName := buildNodeIdToNameMap(graph)
				for _, expected := range tt.Edges {
					assertEdge(t, graph, nodeIdToName, expected)
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
			require.NotNil(t, stringChild.CompareEvidence)
			require.Equal(t, "string", stringChild.CompareEvidence.FilterType)
			require.Equal(t, "have", stringChild.CompareEvidence.MatchMode)

			// 验证 opcode 条件
			require.NotNil(t, opcodeChild.CompareEvidence)
			require.Equal(t, "opcode", opcodeChild.CompareEvidence.FilterType)

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
				require.NotNil(t, child.CompareEvidence)
				require.Equal(t, "string", child.CompareEvidence.FilterType)
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
