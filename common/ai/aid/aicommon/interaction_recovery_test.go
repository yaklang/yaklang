package aicommon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestUIRecoveryUsesReceiptAfterInterruptedSnapshotWrite(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)
	cfg := NewTestConfig(context.Background(), WithPersistentSessionId("ui-session"),
		WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`))
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("ui-runtime", db)
	request := interaction.QuestionRequest{ID: "request", Mode: "async", State: "pending", Questions: uiQuestions()}
	receipt := interaction.Submission{ID: "receipt", Source: "questionnaire", RequestID: request.ID,
		Answers: []interaction.Answer{{QuestionID: "scope", Text: "只读"}}}
	message := UIUserMessage{ID: receipt.ID, Text: "已接受的回答", Metadata: receipt}
	for _, event := range []*schema.AiOutputEvent{
		{SessionId: "ui-session", Type: "ui_question", Content: utils.Jsonify(request)},
		{SessionId: "ui-session", Type: "user_message", Content: utils.Jsonify(message)},
		{SessionId: "another-session", Type: "ui_question", Content: utils.Jsonify(interaction.QuestionRequest{ID: "foreign"})},
	} {
		require.NoError(t, db.Create(event).Error)
	}
	require.NoError(t, cfg.RestoreUIInteractions())
	state := cfg.UISession()
	state.mu.Lock()
	require.Equal(t, "submitted", state.questions["request"].Request.State)
	require.NotContains(t, state.questions, "foreign")
	state.mu.Unlock()
	raw, _ := json.Marshal(receipt)
	calls := 0
	require.NoError(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(raw)}, func(string, string, *Config) (string, error) {
		calls++
		return "", nil
	}))
	require.Zero(t, calls)
	receipt.Answers[0].Text = "different answer with reused id"
	raw, _ = json.Marshal(receipt)
	require.Error(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(raw)}, nil))
}

func TestUIUnknownProtocolFallsBackAndCachedCapabilitiesAreNotReused(t *testing.T) {
	cfg := NewTestConfig(context.Background(), WithClientUI(`{"protocol":"future","questions":true}`))
	require.False(t, cfg.UIQuestionsEnabled())
	cached := &ypb.AIStartParams{ClientUI: `{"protocol":"memfit.ui/1","questions":true}`}
	// The conversion boundary is tested separately; the shared state must also
	// clear support when a connection negotiates no capabilities.
	require.NoError(t, WithClientUI(cached.ClientUI)(cfg))
	require.True(t, cfg.UIQuestionsEnabled())
	require.NoError(t, WithClientUI("")(cfg))
	require.False(t, cfg.UIQuestionsEnabled())
}
