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
	planPromptV2 := `{"@action":"plan","main_task":"v2","main_task_goal":"v2","tasks":[{"subtask_name":"check","subtask_goal":"check","subtask_identifier":"check","depends_on":["v2_missing"]}]}`

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
	}, callbackV1, aicommon.WithAgreeYOLO(true), aicommon.WithDisableDynamicPlanning(true))
	require.ErrorContains(t, err, "plan requires executable tasks")
	require.Nil(t, resultV1)
	require.Nil(t, gotPlanV1, "invalid presets must fail before approval or model execution")

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
	}, callbackV2, aicommon.WithAgreeYOLO(true), aicommon.WithDisableDynamicPlanning(true))
	require.ErrorContains(t, err, "v2_missing", "the updated database definition must be loaded, rather than the previous invalid preset")
	require.Nil(t, resultV2)
	require.Nil(t, gotPlanV2, "invalid DAGs must fail before approval or model execution")
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
