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
	deltaDescription := deltaSchema["description"].(string)
	require.Contains(t, deltaDescription, "The only TODO write channel")
	require.Contains(t, deltaDescription, "each newly observed, relevant independent target with its own ID")
	require.Contains(t, deltaDescription, "keep it OPEN")
	require.Contains(t, deltaDescription, "Closed IDs are immutable")
	require.Contains(t, deltaDescription, "does not prove completion")

	addSchema := deltaSchema["properties"].(map[string]any)["add"].(map[string]any)
	addItemSchema := addSchema["items"].(map[string]any)
	addTextSchema := addItemSchema["properties"].(map[string]any)["text"].(map[string]any)
	require.Contains(t, addTextSchema["description"].(string), "ONE independently verifiable target")
	require.Contains(t, addTextSchema["description"].(string), "source (user request or Observation), and acceptance check")

	closeSchema := deltaSchema["properties"].(map[string]any)["close"].(map[string]any)
	itemSchema := closeSchema["items"].(map[string]any)
	required := itemSchema["required"].([]any)
	require.ElementsMatch(t, []any{"id", "outcome", "reason"}, required)

	reviewSchema := properties["completion_review"].(map[string]any)
	require.ElementsMatch(t, []any{"goal_evidence", "discovery_audit", "closure_audit"}, reviewSchema["required"])
	require.NotContains(t, schema["required"], "completion_review", "ordinary tool/answer actions do not need a finish audit")
}
