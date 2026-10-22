package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

func uiFormFixture(t *testing.T, options ...ConfigOption) (*Config, interaction.ComponentDocument) {
	t.Helper()
	raw, err := os.ReadFile("../interaction/testdata/catalog.json")
	require.NoError(t, err)
	caps, _ := json.Marshal(UIClientCapabilities{Protocol: interaction.Protocol, Questions: true, Catalog: raw})
	cfg := NewTestConfig(context.Background(), append([]ConfigOption{WithClientUI(string(caps))}, options...)...)
	doc := interaction.ComponentDocument{ID: "arch_questions", CatalogVersion: cfg.UICatalog().Version, Fallback: "填写姓名", Root: "form",
		Nodes: []interaction.ComponentNode{{ID: "form", Component: "ParameterForm",
			Props: map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{
				"name": map[string]any{"type": "string"},
			}, "required": []any{"name"}, "additionalProperties": false}},
			Action: &interaction.ComponentAction{Name: "submit_to_chat", Args: map[string]any{}},
		}}}
	return cfg, doc
}

func uiFormReceipt(id, name string) interaction.Submission {
	return interaction.Submission{ID: id, Source: "component", RequestID: "arch_questions", NodeID: "form", Values: map[string]any{"name": name}}
}

func TestUIComponentAcceptsOnlyOneSubmissionAcrossTransportIDsAndRevisions(t *testing.T) {
	var messages int
	cfg, doc := uiFormFixture(t, WithEventHandler(func(event *schema.AiOutputEvent) {
		if event.Type == "user_message" {
			messages++
		}
	}))
	_, err := cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	deliveries := 0
	deliver := func(string, string, *Config) (string, error) { deliveries++; return "", nil }
	var wg sync.WaitGroup
	errors := make(chan error, 15)
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errors <- cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(uiFormReceipt(fmt.Sprint(i), "小明")))}, deliver)
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, 1, deliveries)
	require.Equal(t, 1, messages)
	_, err = cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	require.NoError(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(uiFormReceipt("after-correction", "小明")))}, deliver))
	require.ErrorContains(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(uiFormReceipt("edited", "另一个回答")))}, deliver), "already submitted")
	require.Equal(t, 1, deliveries)
}

func TestUIComponentRecoveryLocksLegacyAndInterruptedSubmissions(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)
	cfg, doc := uiFormFixture(t, WithPersistentSessionId("ui-session"))
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("ui-runtime", db)
	normalized, diagnostics := cfg.UICatalog().Normalize(doc)
	require.Empty(t, diagnostics)
	artifact := UIArtifact{Document: normalized, Catalog: cfg.UICatalog()}
	require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: "ui-session", Type: "ui_component", Content: utils.Jsonify(artifact)}).Error)
	for i := 0; i < 15; i++ {
		name := "小明"
		if i > 0 {
			name = "旧版本重复提交"
		}
		receipt := uiFormReceipt(fmt.Sprint(i), name)
		require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: "ui-session", Type: "user_message",
			Content: utils.Jsonify(UIUserMessage{ID: receipt.ID, Metadata: receipt, Text: name})}).Error)
	}
	require.NoError(t, cfg.RestoreUIInteractions())
	require.Equal(t, "0", cfg.UISession().artifacts[doc.ID].Submissions["form"].ID)
	calls := 0
	deliver := func(string, string, *Config) (string, error) { calls++; return "", nil }
	require.NoError(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(uiFormReceipt("retry", "小明")))}, deliver))
	require.Error(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(uiFormReceipt("change", "不同值")))}, deliver))
	require.Zero(t, calls)
}

func TestUIInteractionPolicyCoversQuestionsFormsAndDisplayCatalog(t *testing.T) {
	cfg, doc := uiFormFixture(t)
	_, err := cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	require.NoError(t, WithAllowRequireForUserInteract(false)(cfg))
	require.False(t, cfg.UIQuestionsEnabled())
	require.Nil(t, cfg.UICatalog().Definition("ParameterForm"))
	require.NotNil(t, cfg.UICatalog().Definition("ComparisonTable"))
	_, err = cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.ErrorContains(t, err, "interaction is disabled")
	require.ErrorContains(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(uiFormReceipt("blocked", "小明")))}, nil), "interaction is disabled")
	_, err = cfg.RequestQuestions(cfg.GetContext(), "task", uiQuestions(), false, 0)
	require.Error(t, err)
	doc.ID, doc.Root = "display", "text"
	doc.Nodes = []interaction.ComponentNode{{ID: "text", Component: "Text", Props: map[string]any{"text": "仍可显示"}}}
	_, err = cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	require.NoError(t, WithAllowRequireForUserInteract(true)(cfg))
	require.True(t, cfg.UIQuestionsEnabled())
	require.NotNil(t, cfg.UICatalog().Definition("ParameterForm"))
}

func uiDataTableFixture(t *testing.T, options ...ConfigOption) (*Config, interaction.ComponentDocument) {
	t.Helper()
	cfg, _ := uiFormFixture(t, options...)
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
	require.ErrorContains(t, err, "interaction is disabled")
	doc.Nodes[4].Action, doc.Nodes[4].Bindings = nil, nil
	doc.Nodes[4].Props["selection"] = "none"
	_, err = cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	require.NoError(t, WithAllowRequireForUserInteract(true)(cfg))
	require.NotEmpty(t, cfg.UICatalog().Definition("DataTable").Actions, "capability filtering must not mutate the original client catalog")
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

func TestUIDataTableSubmissionIsCanonicalOnceAndRecoverable(t *testing.T) {
	var accepted UIUserMessage
	cfg, doc := uiDataTableFixture(t, WithPersistentSessionId("table-session"), WithEventHandler(func(event *schema.AiOutputEvent) {
		if event.Type == "user_message" {
			require.NoError(t, json.Unmarshal(event.Content, &accepted))
		}
	}))
	_, err := cfg.RenderUI(cfg.GetContext(), "task", doc)
	require.NoError(t, err)
	deliveries := 0
	deliver := func(text, task string, origin *Config) (string, error) {
		deliveries++
		require.Contains(t, text, "API 网关")
		require.NotContains(t, text, "forged")
		return "task", nil
	}
	answer := func(id string) *ypb.AIInputEvent {
		return &ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(interaction.Submission{
			ID: id, Source: "component", RequestID: doc.ID, NodeID: "details", Values: map[string]any{"selectedRowIds": []any{"gateway"}, "rows": []any{"forged"}},
		}))}
	}
	for i := 0; i < 12; i++ {
		require.NoError(t, cfg.AcceptQuestionAnswer(answer(fmt.Sprint(i)), deliver))
	}
	// Retries use the original transport ID and omit canonical server row data.
	require.NoError(t, cfg.AcceptQuestionAnswer(answer("0"), deliver))
	require.Equal(t, 1, deliveries)
	require.Equal(t, "0", accepted.ID)
	require.Equal(t, float64(12), accepted.Metadata.Values["rows"].([]any)[0].(map[string]any)["values"].(map[string]any)["count"])
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)
	require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: "table-session", Type: "ui_component", Content: utils.Jsonify(cfg.UISession().artifacts[doc.ID])}).Error)
	require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: "table-session", Type: "user_message", Content: utils.Jsonify(accepted)}).Error)
	recovered, _ := uiDataTableFixture(t, WithPersistentSessionId("table-session"))
	recovered.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("table-runtime", db)
	require.NoError(t, recovered.RestoreUIInteractions())
	require.NoError(t, recovered.AcceptQuestionAnswer(answer("0"), deliver))
	require.NoError(t, recovered.AcceptQuestionAnswer(answer("after-restart"), deliver))
	require.Equal(t, 1, deliveries)
	require.Equal(t, "0", recovered.UISession().artifacts[doc.ID].Submissions["details"].ID)
	require.NoError(t, WithAllowRequireForUserInteract(false)(recovered))
	changed := interaction.Submission{ID: "changed", Source: "component", RequestID: doc.ID, NodeID: "details", Values: map[string]any{"selectedRowIds": []any{"admin"}}}
	require.Error(t, recovered.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(utils.Jsonify(changed))}, deliver))
}

func TestUIInformationDisplayWithQuestionsDisabledAndLegacyFallback(t *testing.T) {
	cfg, _ := uiFormFixture(t, WithAllowRequireForUserInteract(false))
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
