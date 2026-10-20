package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

func uiFormFixture(t *testing.T, options ...ConfigOption) (*Config, interaction.ComponentDocument) {
	t.Helper()
	raw, err := os.ReadFile("../interaction/testdata/catalog.json")
	require.NoError(t, err)
	caps, _ := json.Marshal(UIClientCapabilities{Protocol: interaction.Protocol, Questions: true, Catalog: raw})
	cfg := NewTestConfig(context.Background(), append([]ConfigOption{WithClientUI(string(caps))}, options...)...)
	doc := interaction.ComponentDocument{ID: "arch_questions", CatalogVersion: "1.0", Fallback: "填写姓名", Root: "form",
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
