package aicommon

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
)

func TestUIComponentCorrectionBudgetAndFallback(t *testing.T) {
	raw, err := os.ReadFile("../interaction/testdata/catalog.json")
	require.NoError(t, err)
	client := UIClientCapabilities{Protocol: interaction.Protocol, Questions: true, Catalog: raw}
	caps, _ := json.Marshal(client)
	events := make(chan *schema.AiOutputEvent, 16)
	c := NewTestConfig(context.Background(), WithClientUI(string(caps)), WithEventHandler(func(e *schema.AiOutputEvent) { events <- e }))
	doc := interaction.ComponentDocument{ID: "block", CatalogVersion: "1.0", Fallback: "有用的文本", Root: "text",
		Nodes: []interaction.ComponentNode{{ID: "text", Component: "Text", Props: map[string]any{}}}}
	result, err := c.RenderUI(c.GetContext(), "task", doc)
	require.NoError(t, err)
	require.True(t, result.(map[string]any)["correctionAllowed"].(bool))
	doc.Nodes[0].Props["text"] = "有效"
	result, err = c.RenderUI(c.GetContext(), "task", doc)
	require.NoError(t, err)
	require.False(t, result.(map[string]any)["correctionAllowed"].(bool))
	_, err = c.RenderUI(c.GetContext(), "task", doc)
	require.Error(t, err)
	<-events
	event := <-events
	require.Same(t, event, ProjectUIEvent(event, client))
	fallback := ProjectUIEvent(event, UIClientCapabilities{})
	require.Equal(t, schema.EVENT_TYPE_RESULT, fallback.Type)
	require.Contains(t, string(fallback.Content), "有用的文本")
	require.Equal(t, schema.EventType("ui_component"), event.Type, "projection cannot mutate the journal")
	require.Nil(t, ProjectUIEvent(&schema.AiOutputEvent{Type: "stream", ContentType: interaction.ComponentContentType}, UIClientCapabilities{}))
}
