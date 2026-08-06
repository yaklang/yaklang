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
	require.Contains(t, deltaDescription, "optional only when state truly does not change")
	require.Contains(t, deltaDescription, "in the same normal action that advances the work")
	require.Contains(t, deltaDescription, "Open items form the Frontier")
	require.Contains(t, deltaDescription, "record every concrete in-scope branch")
	require.Contains(t, deltaDescription, "is sufficient source evidence for a coverage TODO")
	require.Contains(t, deltaDescription, "require a falsifiable hypothesis only for a verification claim")
	require.Contains(t, deltaDescription, "A tool/parameter/transport/auth failure or one payload miss is not closure")
	require.Contains(t, deltaDescription, "vary the controllable channel first")
	require.Contains(t, deltaDescription, "set the next Frontier item in the same delta")

	addSchema := deltaSchema["properties"].(map[string]any)["add"].(map[string]any)
	addItemSchema := addSchema["items"].(map[string]any)
	addTextSchema := addItemSchema["properties"].(map[string]any)["text"].(map[string]any)
	require.Contains(t, addTextSchema["description"].(string), "concrete target, source evidence, and first resume action")
	require.Contains(t, addTextSchema["description"].(string), "falsifiable hypothesis when the item verifies a claim")

	closeSchema := deltaSchema["properties"].(map[string]any)["close"].(map[string]any)
	itemSchema := closeSchema["items"].(map[string]any)
	required := itemSchema["required"].([]any)
	require.ElementsMatch(t, []any{"id", "outcome", "reason"}, required)
}
