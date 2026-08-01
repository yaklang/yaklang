package reactloops

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTodoDeltaIsSharedOptionalFieldNotStandaloneAction(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(buildSchema(loopAction_Finish, loopAction_DirectlyAnswer)), &schema))
	properties := schema["properties"].(map[string]any)
	require.Contains(t, properties, "todo_delta")

	actionSchema := properties["@action"].(map[string]any)
	for _, value := range actionSchema["enum"].([]any) {
		require.NotEqual(t, "todo_delta", value, "todo_delta must not be registered as an independent action")
	}

	deltaSchema := properties["todo_delta"].(map[string]any)
	closeSchema := deltaSchema["properties"].(map[string]any)["close"].(map[string]any)
	itemSchema := closeSchema["items"].(map[string]any)
	required := itemSchema["required"].([]any)
	require.ElementsMatch(t, []any{"id", "outcome", "reason"}, required)
}
