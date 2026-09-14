package aireact

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestRestoreInitializedSessionTitleIgnoresPlaceholder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var emittedTitles []string
	cfg := aicommon.NewConfig(
		ctx,
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event != nil && event.NodeId == "session_title" {
				emittedTitles = append(emittedTitles, string(event.Content))
			}
		}),
	)
	react := &ReAct{config: cfg, Emitter: cfg.Emitter}

	restored := react.restoreInitializedSessionTitle(&schema.AISession{
		Title:            "<未命名>",
		TitleInitialized: false,
	})

	require.False(t, restored)
	require.Empty(t, cfg.GetConfigString("session_title", ""))
	require.False(t, cfg.GetConfigBool(sessionTitleGeneratedKey))
	require.Empty(t, emittedTitles)
}

func TestRestoreInitializedSessionTitlePublishesDurableTitle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var emittedTitles []string
	cfg := aicommon.NewConfig(
		ctx,
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event != nil && event.NodeId == "session_title" {
				emittedTitles = append(emittedTitles, string(event.Content))
			}
		}),
	)
	react := &ReAct{config: cfg, Emitter: cfg.Emitter}

	restored := react.restoreInitializedSessionTitle(&schema.AISession{
		Title:            "  已生成的会话标题  ",
		TitleInitialized: true,
	})

	require.True(t, restored)
	require.Equal(t, "已生成的会话标题", cfg.GetConfigString("session_title", ""))
	require.Equal(t, "已生成的会话标题", cfg.GetSessionTitle())
	require.True(t, cfg.GetConfigBool(sessionTitleGeneratedKey))
	require.Len(t, emittedTitles, 1)
}

// This is the production combination that regressed when work-directory
// naming became opt-in: persistent sessions already have an uninitialized
// "<未命名>" row, while display-title naming runs asynchronously.
func TestPersistentSessionPlaceholderDoesNotBlockAsyncTitleGeneration(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}, &schema.AiCheckpoint{}).Error)

	const sessionID = "persistent-placeholder-title"
	meta, err := yakit.EnsureAISessionMeta(db, sessionID)
	require.NoError(t, err)
	require.Equal(t, "<未命名>", meta.Title)
	require.False(t, meta.TitleInitialized)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	titleEmitted := make(chan struct{}, 1)
	react, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithDisableSessionTitleGeneration(false),
		aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedLoopDirectlyAnswerOutput(c, `{"@action":"session-title-generator","session_title":"持久化会话标题"}`)
		}),
	)
	require.NoError(t, err)
	defer func() {
		cancel()
		react.Wait()
	}()

	// NewTestReAct is deliberately created without a persistent ID so the test
	// can inject its isolated DB without rebinding process-global test state.
	react.config.PersistentSessionId = sessionID
	react.config.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(react.config.GetRuntimeId(), db)
	testEmitter := aicommon.NewEmitter("persistent-title-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event != nil && event.NodeId == "session_title" {
			select {
			case titleEmitted <- struct{}{}:
			default:
			}
		}
		return event, nil
	})
	react.Emitter = testEmitter
	react.config.Emitter = testEmitter

	require.False(t, react.restoreInitializedSessionTitle(meta))
	require.False(t, react.config.GetConfigBool(sessionTitleGeneratedKey))
	react.ensureSessionTitle("检查 Web 应用中的 SQL 注入")

	select {
	case <-titleEmitted:
	case <-ctx.Done():
		t.Fatal("asynchronous persistent-session title generation did not finish")
	}
	saved, err := yakit.GetAISessionMetaBySessionID(db, sessionID)
	require.NoError(t, err)
	require.Equal(t, "持久化会话标题", saved.Title)
	require.True(t, saved.TitleInitialized)
}
