package coordinator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPresetPlanDuplicateDisplayNames(t *testing.T) {
	const raw = `{"@action":"plan","query":"-","main_task":"生成安全运营报告","main_task_goal":"读取当次输入的运营数据，按技能体内置结构输出报告。","tasks":[{"subtask_name":"生成安全运营报告","subtask_goal":"完整解析输入并输出报告，不编造数据。"}]}`
	p, err := ParsePlan(raw, "", nil)
	require.NoError(t, err)
	require.Len(t, p.Tasks, 1)
	var root PlanNode
	require.NoError(t, json.Unmarshal(p.Tree, &root))
	require.Equal(t, root.Name, root.Subtasks[0].Name)
	require.NotEqual(t, root.Identifier, root.Subtasks[0].Identifier)
	next, err := ParsePlan(string(p.Tree), "", p)
	require.NoError(t, err)
	require.Equal(t, p.Tasks[0].ID, next.Tasks[0].ID)
}

func TestPresetPlanGeneratedIdentifiersAvoidExplicitAndNaturalNames(t *testing.T) {
	raw := `{"name":"Root","goal":"Report","tasks":[{"name":"Report","goal":"First"},{"name":"Report","goal":"Second"},{"name":"Report-2","goal":"Third"},{"name":"Explicit","goal":"Fourth","identifier":"report"}]}`
	p, err := ParsePlan(raw, "", nil)
	require.NoError(t, err)
	var root PlanNode
	require.NoError(t, json.Unmarshal(p.Tree, &root))
	seen := map[string]bool{}
	root.walk(func(n *PlanNode) { require.False(t, seen[n.Identifier]); seen[n.Identifier] = true })
	require.Equal(t, "report", root.Subtasks[3].Identifier)
	require.Equal(t, "report_2", root.Subtasks[2].Identifier)
	// Display names are not silently resolved when more than one task matches.
	_, err = ParsePlan(`{"name":"Root","goal":"Report","tasks":[{"name":"Same","goal":"First"},{"name":"Same","goal":"Second"},{"name":"Final","goal":"Third","depends_on":["Same"]}]}`, "", nil)
	require.Error(t, err)
}
