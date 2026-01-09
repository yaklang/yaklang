package aiforge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestExecuteForgeAndAutoRegister_UsesLatestDBForge(t *testing.T) {
	setupTestDatabase(t)

	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	forgeName := "test-forge-db-exec-" + utils.RandStringBytes(6)
	defer func() {
		_ = yakit.DeleteAIForgeByName(db, forgeName)
	}()

	cliCode := `query = cli.String("query", cli.setRequired(true))`

	planPromptV1 := `{"@action":"plan","main_task":"v1","main_task_goal":"v1","tasks":[]}`
	planPromptV2 := `{"@action":"plan","main_task":"v2","main_task_goal":"v2","tasks":[]}`

	forge := &schema.AIForge{
		ForgeName:        forgeName,
		ForgeType:        "yak",
		Params:           cliCode,
		ForgeContent:     cliCode,
		InitPrompt:       "INIT_TOKEN_V1",
		PersistentPrompt: "PERSIST_TOKEN_V1",
		PlanPrompt:       planPromptV1,
		ResultPrompt:     "RESULT_TOKEN_V1",
		Actions:          "action_v1",
	}
	require.NoError(t, yakit.CreateAIForge(db, forge))

	var gotPlanV1 *planReviewEvent
	callbackV1 := aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
		if event.Type != schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
			return
		}
		gotPlanV1 = decodePlanReviewEvent(t, event.Content)
	})

	resultV1, err := aicommon.ExecuteRegisteredForge(forgeName, context.Background(), []*ypb.ExecParamItem{
		{Key: "query", Value: "hello"},
	}, callbackV1, aicommon.WithAgreeYOLO(true))
	require.ErrorContains(t, err, "no subtasks")
	require.Nil(t, resultV1)
	require.NotNil(t, gotPlanV1)
	assertPlanMatchesPrompt(t, planPromptV1, gotPlanV1)

	updated := &schema.AIForge{
		ForgeName:        forgeName,
		ForgeType:        "yak",
		Params:           cliCode,
		ForgeContent:     cliCode,
		InitPrompt:       "INIT_TOKEN_V2",
		PersistentPrompt: "PERSIST_TOKEN_V2",
		PlanPrompt:       planPromptV2,
		ResultPrompt:     "RESULT_TOKEN_V2",
		Actions:          "action_v2",
	}
	require.NoError(t, yakit.UpdateAIForgeByName(db, forgeName, updated))

	var gotPlanV2 *planReviewEvent
	callbackV2 := aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
		if event.Type != schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
			return
		}
		gotPlanV2 = decodePlanReviewEvent(t, event.Content)
	})

	resultV2, err := ExecuteForgeAndAutoRegister(forgeName, context.Background(), []*ypb.ExecParamItem{
		{Key: "query", Value: "hello"},
	}, callbackV2, aicommon.WithAgreeYOLO(true))
	require.ErrorContains(t, err, "no subtasks")
	require.Nil(t, resultV2)
	require.NotNil(t, gotPlanV2)
	assertPlanMatchesPrompt(t, planPromptV2, gotPlanV2)
}

type planPromptPayload struct {
	MainTask     string `json:"main_task"`
	MainTaskGoal string `json:"main_task_goal"`
	Tasks        []struct {
		SubtaskName string `json:"subtask_name"`
		SubtaskGoal string `json:"subtask_goal"`
	} `json:"tasks"`
}

type planReviewEvent struct {
	Plans struct {
		RootTask struct {
			Name     string `json:"name"`
			Goal     string `json:"goal"`
			Subtasks []struct {
				Name string `json:"name"`
				Goal string `json:"goal"`
			} `json:"subtasks"`
		} `json:"root_task"`
	} `json:"plans"`
}

func decodePlanReviewEvent(t *testing.T, content []byte) *planReviewEvent {
	t.Helper()
	var payload planReviewEvent
	require.NoError(t, json.Unmarshal(content, &payload))
	return &payload
}

func assertPlanMatchesPrompt(t *testing.T, planPrompt string, event *planReviewEvent) {
	t.Helper()
	var expected planPromptPayload
	require.NoError(t, json.Unmarshal([]byte(planPrompt), &expected))
	require.Equal(t, expected.MainTask, event.Plans.RootTask.Name)
	require.Equal(t, expected.MainTaskGoal, event.Plans.RootTask.Goal)
	require.Equal(t, len(expected.Tasks), len(event.Plans.RootTask.Subtasks))
	for idx, task := range expected.Tasks {
		require.Equal(t, task.SubtaskName, event.Plans.RootTask.Subtasks[idx].Name)
		require.Equal(t, task.SubtaskGoal, event.Plans.RootTask.Subtasks[idx].Goal)
	}
}
