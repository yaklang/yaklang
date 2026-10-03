package coordinator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const nestedPresetPlan = `{
 "main_task":"Audit","main_task_goal":"Read, compare and report","main_task_identifier":"audit",
 "tasks":[
  {"subtask_name":"Scope","subtask_goal":"Confirm boundaries","subtask_identifier":"scope","depends_on":[]},
  {"subtask_name":"Sources","subtask_goal":"Inspect sources","subtask_identifier":"sources","depends_on":["scope"],"sub_subtasks":[
   {"subtask_name":"First","subtask_goal":"Read first source","subtask_identifier":"first","depends_on":[]},
   {"subtask_name":"Second","subtask_goal":"Compare with first source","subtask_identifier":"second","depends_on":["first"]}
  ]},
  {"subtask_name":"Report","subtask_goal":"Report verified results","subtask_identifier":"report","depends_on":["sources"]}
 ]
}`

func TestCoordinatorHistoricalNestedPlanFunctionContract(t *testing.T) {
	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"plan":`+nestedPresetPlan+`}`), &args))
	tool := aitool.NewWithoutCallback("create_plan", planParameter())
	valid, problems := tool.ValidateParams(args)
	require.True(t, valid, problems)
	// Nested nodes must be validated too; a partial brief cannot silently pass
	// merely because the parent array contains valid objects.
	args["plan"].(map[string]any)["tasks"].([]any)[1].(map[string]any)["sub_subtasks"].([]any)[0].(map[string]any)["subtask_goal"] = nil
	valid, _ = tool.ValidateParams(args)
	require.False(t, valid)
}

func TestCoordinatorApprovedContextSupersedesPreviousDocument(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Session{Config: aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true))}
	p, err := ParsePlan(nestedPresetPlan, "previous document must disappear", nil)
	require.NoError(t, err)
	snapshot := Snapshot{Phase: PhaseExec, Plan: p, Attempts: map[string]Attempt{}}
	for _, task := range p.Tasks {
		snapshot.Attempts[task.ID] = Attempt{Task: task, State: Pending}
	}
	s.Changed(snapshot)
	replacement := *p
	replacement.Document = "updated document"

	snapshot.Plan = &replacement
	s.Changed(snapshot)
	partitions := s.GetOrCreateFrozenBlockPartitionProducer().ProducePartitions()
	var document, definition string
	for _, partition := range partitions {
		switch partition.ID {
		case "plan_document":
			document = partition.Content
		case "plan_definition":
			definition = partition.Content
		}
	}
	require.Contains(t, document, "updated document")
	require.NotContains(t, document, "previous document must disappear")
	require.Contains(t, definition, "PLAN DEFINITION")
	require.NotContains(t, definition, "version")
}

func TestCoordinatorTaskStateChangesKeepPlanPartitionsStable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Session{Config: aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true))}
	p, err := ParsePlan(nestedPresetPlan, "immutable plan document", nil)
	require.NoError(t, err)
	snapshot := Snapshot{Phase: PhaseExec, Plan: p, Attempts: map[string]Attempt{}}
	for _, task := range p.Tasks {
		snapshot.Attempts[task.ID] = Attempt{Task: task, State: Pending}
	}
	s.Changed(snapshot)
	before := s.GetOrCreateFrozenBlockPartitionProducer().ProducePartitions()
	changed := Snapshot{Phase: PhaseExec, Plan: p, Attempts: map[string]Attempt{}}
	for id, attempt := range snapshot.Attempts {
		changed.Attempts[id] = attempt
	}
	a := changed.Attempts[p.Tasks[0].ID]
	a.ID, a.State = 1, AwaitingReview
	a.Result = Result{Summary: "new volatile result", EvidenceIDs: []string{"verified.source"}}
	changed.Attempts[p.Tasks[0].ID] = a
	s.Changed(changed)
	require.Equal(t, before, s.GetOrCreateFrozenBlockPartitionProducer().ProducePartitions(), "task result/status must not rewrite plan content or its partition nonce")
	a.State = Accepted
	changed.Attempts[p.Tasks[0].ID] = a
	s.Changed(changed)
	require.Equal(t, before, s.GetOrCreateFrozenBlockPartitionProducer().ProducePartitions(), "review must preserve the stable plan partitions")
}

func TestCoordinatorNestedDAGRetainsGroupEntrySemantics(t *testing.T) {
	p, err := ParsePlan(nestedPresetPlan, "Stable document", nil)
	require.NoError(t, err)
	require.Len(t, p.Tasks, 4, "structural groups must never execute")
	scope, first, second, report := p.Tasks[0], p.Tasks[1], p.Tasks[2], p.Tasks[3]
	require.Empty(t, scope.DependsOn)
	require.Equal(t, []string{scope.ID}, first.DependsOn)
	require.Equal(t, []string{first.ID}, second.DependsOn, "group prerequisite belongs to entry leaves, not every descendant")
	require.Equal(t, []string{first.ID, second.ID}, report.DependsOn, "group dependency waits for all leaves")
	require.Equal(t, "2-1", first.Index)
	updated, err := ParsePlan(nestedPresetPlan, "Updated document", p)
	require.NoError(t, err)
	for i := range p.Tasks {
		require.Equal(t, p.Tasks[i].ID, updated.Tasks[i].ID)
	}
	s := Snapshot{Phase: PhaseExec, Plan: p, Attempts: map[string]Attempt{}}
	for _, task := range p.Tasks {
		s.Attempts[task.ID] = Attempt{Task: task, State: Pending}
	}
	definition := s.PlanDefinition()
	require.Contains(t, definition, "Compare with first source")
	require.NotContains(t, definition, `"progress"`)
	s.Attempts[scope.ID] = Attempt{Task: scope, State: Accepted, ID: 9, Result: Result{Summary: "volatile result"}}
	require.Equal(t, definition, s.PlanDefinition(), "execution must not change cacheable plan definitions")
	require.Contains(t, s.PromptStatus(), "Dispatch: ready")
	require.Contains(t, s.PromptStatus(), "Dispatch: blocked")
	require.NotContains(t, s.PromptStatus(), "Compare with first source")
}
