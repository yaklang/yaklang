package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestUIClientCatalogGrowthHasSeparateTransportLimit(t *testing.T) {
	catalog := interaction.Catalog{Version: "large"}
	for i := 0; i < interaction.MaxCatalogComponents; i++ {
		catalog.Components = append(catalog.Components, interaction.ComponentDefinition{
			Name: fmt.Sprintf("Component%d", i), Description: strings.Repeat("description ", 16),
			Schema: map[string]any{"type": "object"},
		})
	}
	raw, err := json.Marshal(catalog)
	require.NoError(t, err)
	caps, err := json.Marshal(UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw})
	require.NoError(t, err)
	require.Greater(t, len(caps), 64*1024)
	cfg := NewTestConfig(context.Background())
	require.NoError(t, WithClientUI(string(caps))(cfg))
	require.Len(t, cfg.UICatalog().Components, interaction.MaxCatalogComponents)
	require.ErrorContains(t, WithClientUI(strings.Repeat(" ", 1024*1024+1))(cfg), "1 MiB")
	require.NoError(t, WithClientUI(`{"protocol":"unsupported","catalog":{}}`)(cfg))
	require.Empty(t, cfg.UICatalog().Components)
}

func TestUIComponentCorrectionBudgetAndFallback(t *testing.T) {
	raw, err := os.ReadFile("../interaction/testdata/catalog.json")
	require.NoError(t, err)
	client := UIClientCapabilities{Protocol: interaction.Protocol, Questions: true, Catalog: raw}
	caps, _ := json.Marshal(client)
	events := make(chan *schema.AiOutputEvent, 16)
	c := NewTestConfig(context.Background(), WithClientUI(string(caps)), WithEventHandler(func(e *schema.AiOutputEvent) { events <- e }))
	doc := interaction.ComponentDocument{ID: "block", CatalogVersion: c.UICatalog().Version, Fallback: "有用的文本", Root: "text",
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

func uiDisplayFixture(t *testing.T, options ...ConfigOption) (*Config, interaction.ComponentDocument) {
	t.Helper()
	raw, err := os.ReadFile("../interaction/testdata/catalog.json")
	require.NoError(t, err)
	caps, _ := json.Marshal(UIClientCapabilities{Protocol: interaction.Protocol, Questions: true, Catalog: raw})
	cfg := NewTestConfig(context.Background(), append([]ConfigOption{WithClientUI(string(caps))}, options...)...)
	doc := interaction.ComponentDocument{ID: "display", CatalogVersion: cfg.UICatalog().Version, Fallback: "显示数据", Root: "text",
		Nodes: []interaction.ComponentNode{{ID: "text", Component: "Text", Props: map[string]any{"text": "显示数据"}}}}
	return cfg, doc
}

func uiDataTableFixture(t *testing.T, options ...ConfigOption) (*Config, interaction.ComponentDocument) {
	t.Helper()
	cfg, _ := uiDisplayFixture(t, options...)
	raw, err := os.ReadFile("../interaction/testdata/visualizations.json")
	require.NoError(t, err)
	var doc interaction.ComponentDocument
	require.NoError(t, json.Unmarshal(raw, &doc))
	return cfg, doc
}

func TestUIDataTablePolicyAndVersionFallback(t *testing.T) {
	cfg, doc := uiDataTableFixture(t, WithAllowRequireForUserInteract(false))
	catalog := cfg.UICatalog()
	entry := catalog.Definition("DataTable")
	require.NotNil(t, entry)
	require.Empty(t, entry.Actions)
	require.Empty(t, entry.Bindings)
	require.NotNil(t, catalog.Definition("BarChart"))
	require.NotNil(t, catalog.Definition("LineChart"))
	_, err := cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	require.NoError(t, WithAllowRequireForUserInteract(true)(cfg))
	require.Empty(t, cfg.UICatalog().Definition("DataTable").Actions)
	event := &schema.AiOutputEvent{Type: "ui_component", Content: utils.Jsonify(UIArtifact{Document: doc})}
	legacy := cfg.UICatalog()
	legacy.Version = "1.0"
	raw, _ := json.Marshal(legacy)
	projected := ProjectUIEvent(event, UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw})
	require.Equal(t, schema.EVENT_TYPE_RESULT, projected.Type)
	require.Contains(t, string(projected.Content), doc.Fallback)
	legacy.Version = doc.CatalogVersion
	legacy.Components = legacy.Components[1:]
	raw, _ = json.Marshal(legacy)
	require.Equal(t, schema.EVENT_TYPE_RESULT, ProjectUIEvent(event, UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw}).Type)
}

func TestUIInformationDisplayWithQuestionsDisabledAndLegacyFallback(t *testing.T) {
	cfg, _ := uiDisplayFixture(t, WithAllowRequireForUserInteract(false))
	raw, err := os.ReadFile("../interaction/testdata/information.json")
	require.NoError(t, err)
	var doc interaction.ComponentDocument
	require.NoError(t, json.Unmarshal(raw, &doc))
	// Remove the parameter view; local browsing is still allowed without questions.
	doc.Nodes[4].Props["items"] = doc.Nodes[4].Props["items"].([]any)[:1]
	doc.Nodes = append(doc.Nodes[:6], doc.Nodes[7:]...)
	for _, name := range []string{"Alert", "Tabs", "Tooltip", "Popover"} {
		require.NotNil(t, cfg.UICatalog().Definition(name))
	}
	doc.RequiredComponents = nil
	_, err = cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	artifact := cfg.UISession().artifacts[doc.ID]
	require.Empty(t, artifact.Diagnostics)
	event := &schema.AiOutputEvent{Type: "ui_component", Content: utils.Jsonify(artifact)}
	legacy := cfg.UICatalog()
	legacy.Version = "1.1"
	raw, _ = json.Marshal(legacy)
	require.Equal(t, schema.EVENT_TYPE_RESULT, ProjectUIEvent(event, UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw}).Type)
}

func TestUIInformationUpgradePreservesOlderDocuments(t *testing.T) {
	cfg, doc := uiDataTableFixture(t)
	doc.CatalogVersion = "1.1"
	catalog := cfg.UICatalog()
	catalog.CompatibleVersions = []string{"1.1"}
	normalized, diagnostics := catalog.Normalize(doc)
	require.Empty(t, diagnostics)
	require.NotEmpty(t, normalized.Nodes)
	event := &schema.AiOutputEvent{Type: "ui_component", Content: utils.Jsonify(UIArtifact{Document: normalized})}
	raw, _ := json.Marshal(catalog)
	require.Same(t, event, ProjectUIEvent(event, UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw}))
	// No compatibility declaration means an exact version match remains required.
	catalog.CompatibleVersions = nil
	raw, _ = json.Marshal(catalog)
	require.Equal(t, schema.EVENT_TYPE_RESULT, ProjectUIEvent(event, UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw}).Type)
	doc.RequiredComponents = append(doc.RequiredComponents, "FutureWidget")
	normalized, diagnostics = cfg.UICatalog().Normalize(doc)
	require.Empty(t, normalized.Nodes)
	require.NotEmpty(t, diagnostics)
}
