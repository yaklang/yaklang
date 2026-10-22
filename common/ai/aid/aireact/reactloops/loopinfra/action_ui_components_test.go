package loopinfra

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

func TestUIComponentActionsValidateRequests(t *testing.T) {
	raw, err := os.ReadFile("../../../interaction/testdata/catalog.json")
	require.NoError(t, err)
	caps, _ := json.Marshal(aicommon.UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw})
	ctx := context.Background()
	cfg := aicommon.NewConfig(ctx, aicommon.WithClientUI(string(caps)))
	loop := reactloops.NewMinimalReActLoop(cfg, newTestInvoker(ctx))
	for _, test := range []struct {
		action   *reactloops.LoopAction
		params   string
		expected string
	}{
		{loadUIComponentsAction, `"names":["BarChart","BarChart"]`, `"status":"loaded"`},
		{loadUIComponentsAction, `"names":[]`, `"names":[]`},
		{loadUIComponentsAction, `"names":"BarChart"`, "names must be an array"},
		{loadUIComponentsAction, `"names":[42]`, "names must be an array"},
		{loadUIComponentsAction, `"names":null`, "names must be an array"},
		{loadUIComponentsAction, `"unrelated":1`, "names must be an array"},
		{loadUIComponentsAction, `"names":["unknown"]`, "component unavailable"},
		{searchUIComponentsAction, `"query":"barchart"`, `"name":"BarChart"`},
		{searchUIComponentsAction, `"query":"","offset":512`, `"matches":[]`},
		{searchUIComponentsAction, `"query":4`, "query must be a string"},
		{searchUIComponentsAction, `"query":"","offset":-1`, "query must be a string"},
		{searchUIComponentsAction, `"query":"","offset":0.5`, "query must be a string"},
		{searchUIComponentsAction, `"query":"","offset":"0"`, "query must be a string"},
	} {
		t.Run(test.params, func(t *testing.T) {
			action := parseToolBatchPromptExample(t, `{"@action":"`+test.action.ActionType+`",`+test.params+`}`, test.action.ActionType)
			op := reactloops.NewActionHandlerOperator(newTestTask(ctx))
			test.action.ActionHandler(loop, action, op)
			require.True(t, op.IsContinued())
			feedback := op.GetFeedback().String()
			require.Contains(t, feedback, test.expected)
			require.NotContains(t, feedback, `"schema":`, "full definitions never go into action history")
			require.LessOrEqual(t, ytoken.CalcTokenCount(feedback), interaction.UISearchTokenBudget)
		})
	}
}
