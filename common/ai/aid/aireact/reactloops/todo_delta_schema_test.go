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
	require.Contains(t, deltaDescription, "The only write channel")
	require.Contains(t, deltaDescription, "mandatory in the same normal action whenever TODO state is initialized")
	require.Contains(t, deltaDescription, "initialize all known work in the first executable action")
	require.Contains(t, deltaDescription, "Open TODOs form the Frontier")
	require.Contains(t, deltaDescription, "record every qualified branch")
	require.Contains(t, deltaDescription, "its tool need not be exposed this turn")
	require.Contains(t, deltaDescription, "Keep current depth-first")
	require.Contains(t, deltaDescription, "close the old item and set the next current in the same delta")

	addSchema := deltaSchema["properties"].(map[string]any)["add"].(map[string]any)
	addItemSchema := addSchema["items"].(map[string]any)
	addTextSchema := addItemSchema["properties"].(map[string]any)["text"].(map[string]any)
	require.Contains(t, addTextSchema["description"].(string), "concrete target, triggering evidence, falsifiable hypothesis, and first resume action")

	closeSchema := deltaSchema["properties"].(map[string]any)["close"].(map[string]any)
	itemSchema := closeSchema["items"].(map[string]any)
	required := itemSchema["required"].([]any)
	require.ElementsMatch(t, []any{"id", "outcome", "reason"}, required)
}
