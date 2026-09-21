package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestLegionForgePresetObservationPlanParsesWithoutModel(t *testing.T) {
	const name = "受限 DNS 观测"
	const goal = "调用 dns_lookup 观察已授权目标及 labels 中每个子域；仅根据实际工具结果生成报告。"
	raw, err := json.Marshal(map[string]any{
		"@action": "plan", "query": "-", "main_task": name, "main_task_goal": goal,
		"tasks": []map[string]string{{"subtask_name": "执行实际观测", "subtask_goal": goal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, prompt string
		valid        bool
	}{
		{"preset_json", string(raw), true},
		{"prose_is_not_a_preset_plan", goal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := testLegionContextForgeRelease(t)
			release.Name = name
			release.PlanPrompt = tc.prompt
			rehashLegionContextForgeRelease(t, release)
			_, blueprint, params, err := buildContextForgeBlueprint(release)
			if err != nil {
				t.Fatal(err)
			}
			if blueprint.PlanMocker == nil {
				t.Fatal("nonempty PlanPrompt did not install the preset plan parser")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := restrictedLegionForgeToolOptions(nil)
			opts = append(opts, aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				t.Error("preset plan parsing must not invoke a model")
				return nil, fmt.Errorf("unexpected model invocation")
			}))
			coordinator, err := blueprint.CreateCoordinatorWithQueryAndParams(ctx, "观察用户已授权目标", params, opts...)
			if err != nil {
				t.Fatal(err)
			}
			if coordinator.PlanMocker == nil {
				t.Fatal("coordinator lost the release preset plan")
			}
			plan := coordinator.PlanMocker(coordinator)
			if !tc.valid {
				if plan != nil {
					t.Fatal("natural-language prose unexpectedly parsed as a preset plan")
				}
				return
			}
			if plan == nil || plan.RootTask == nil {
				t.Fatal("valid observation plan did not parse")
			}
			root := plan.RootTask
			if root.Name != name || root.Goal != goal || len(root.Subtasks) != 1 {
				t.Fatalf("preset root or task count changed: %+v", root)
			}
			task := root.Subtasks[0]
			if task.Name != "执行实际观测" || task.Goal != goal {
				t.Fatalf("observation task lost its execution goal: %+v", task)
			}
		})
	}
}
