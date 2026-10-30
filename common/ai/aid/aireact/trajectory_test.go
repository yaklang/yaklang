package aireact

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReActTrajectoryKeepsOnePromptPerTaskIncludingCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}, &schema.AITrajectory{}).Error)
	cfg := aicommon.NewConfig(ctx, aicommon.WithSingleAIModelMode(true), aicommon.WithDisableAutoSkills(true),
		aicommon.WithPersistentSessionId("task-trajectory-session"), aicommon.WithNoOpMemoryTriage())
	cfg.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(cfg.GetRuntimeId(), db)
	cfg.Timeline = aicommon.NewTimeline(nil, nil)
	cfg.Timeline.SoftBindConfig(cfg, cfg)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: cfg.GetRuntimeId(), PersistentSession: cfg.PersistentSessionId}).Error)
	react := &ReAct{config: cfg}
	for i, id := range []string{"first", "second", "cancelled"} {
		task := aicommon.NewStatefulTaskBase(id, id+" question", ctx, nil)
		aicommon.RecordTaskTrajectoryPrompt(task, "exact prompt for "+id)
		if id == "cancelled" {
			task.SetUserCancelled()
			task.SetStatus(aicommon.AITaskState_Skipped)
			cancel()
		} else {
			task.SetStatus(aicommon.AITaskState_Completed)
		}
		react.collectTerminalTrajectory(task)
		react.collectTerminalTrajectory(task)
		require.Eventually(t, func() bool {
			rows, err := yakit.QueryAITrajectories(db, cfg.PersistentSessionId)
			return err == nil && len(rows) == i+1 && rows[i].Prompt == "exact prompt for "+id
		}, 3*time.Second, time.Millisecond)
	}
	rows, err := yakit.QueryAITrajectories(db, cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, "skipped", rows[2].Status)
	for _, row := range rows {
		require.Empty(t, row.Response)
	}
}

func TestReActTrajectoryQueueCapturesTerminalPrompt(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "native"}[native], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			sessionID := "trajectory-e2e-" + uuid.NewString()
			calls := 0
			var finalPrompt, compressionPrompt atomic.Pointer[string]
			runtime, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithSingleAIModelMode(true),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithAgreeYOLO(), aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithPersistentSessionId(sessionID), aicommon.WithEventHandler(func(*schema.AiOutputEvent) {}),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						return guiRespond(c, req, false, guiReplayStep{"verify-satisfaction", aitool.InvokeParams{"user_satisfied": true, "reasoning": "answer delivered"}})
					}
					if !native {
						// Complete the same provider receipt that production text
						// transports deliver alongside the model output stream.
						wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						wire.FinishReasonCallback("stop", nil)
					}
					calls++
					if calls == 1 {
						prompt := req.GetPrompt()
						compressionPrompt.Store(&prompt)
						return guiRespond(c, req, native, guiReplayStep{"compress_context", aitool.InvokeParams{
							"summary": "EXACT_COMPRESSION_SUMMARY", "retain_timeline": "",
							"retire_skill_names": []string{}, "retire_evidence_ids": []string{},
						}})
					}
					if calls == 2 {
						return guiRespond(c, req, native, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": "EXACT_TRAJECTORY_ANSWER"}})
					}
					prompt := req.GetPrompt()
					finalPrompt.Store(&prompt)
					return guiRespond(c, req, native, guiReplayStep{"finish", aitool.InvokeParams{}})
				}))
			require.NoError(t, err)
			defer func() {
				if t.Failed() {
					rows, err := yakit.QueryAITrajectories(runtime.config.GetDB(), sessionID)
					t.Logf("trajectory count=%d query_error=%v", len(rows), err)
					for _, row := range rows {
						finalMatch := finalPrompt.Load() != nil && row.Prompt == *finalPrompt.Load()
						compressionMatch := compressionPrompt.Load() != nil && row.Prompt == *compressionPrompt.Load()
						t.Logf("kind=%s status=%s prompt_bytes=%d response_bytes=%d final_match=%t compression_match=%t", row.Kind, row.Status, len(row.Prompt), len(row.Response), finalMatch, compressionMatch)
					}
				}
				cancel()
				runtime.WaitLifecycleStopped()
				_, _, _ = yakit.DeleteAISession(runtime.config.GetDB(), sessionID)
			}()
			runtime.config.Timeline.PushText(runtime.config.AcquireId(), "history to compress")
			require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Explain this test trajectory"}))

			require.Eventually(t, func() bool {
				rows, err := yakit.QueryAITrajectories(runtime.config.GetDB(), sessionID)
				return err == nil && len(rows) == 2 && rows[0].Kind == schema.AITrajectoryCompression &&
					compressionPrompt.Load() != nil && rows[0].Prompt == *compressionPrompt.Load() &&
					strings.Contains(rows[0].Response, "EXACT_COMPRESSION_SUMMARY") && rows[1].Kind == schema.AITrajectoryTerminal &&
					rows[1].Response == "" && finalPrompt.Load() != nil && rows[1].Prompt == *finalPrompt.Load() && rows[1].Status == "completed" &&
					strings.Contains(rows[1].Prompt, "Explain this test trajectory") && len(rows[1].Prompt) > 100
			}, 12*time.Second, 20*time.Millisecond)
		})
	}
}

func TestReActTrajectoryUserCancelCapturesInFlightPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sessionID := "trajectory-cancel-" + uuid.NewString()
	started := make(chan string, 1)
	runtime, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithSingleAIModelMode(true),
		aicommon.WithAgreeYOLO(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithEventHandler(func(*schema.AiOutputEvent) {}),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			started <- req.GetPrompt()
			<-req.GetContext().Done()
			return nil, req.GetContext().Err()
		}))
	require.NoError(t, err)
	defer func() {
		cancel()
		runtime.WaitLifecycleStopped()
		_, _, _ = yakit.DeleteAISession(runtime.config.GetDB(), sessionID)
	}()
	require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Run until I cancel this trajectory"}))
	var prompt string
	select {
	case prompt = <-started:
	case <-ctx.Done():
		t.Fatal("model request did not start")
	}
	task := runtime.getProcessingRuntimeTask()
	require.NotNil(t, task)
	runtime.CancelTask(task, &ypb.AIInputEvent{})
	require.Eventually(t, func() bool {
		rows, err := yakit.QueryAITrajectories(runtime.config.GetDB(), sessionID)
		return err == nil && len(rows) == 1 && rows[0].Status == "skipped" && rows[0].Prompt == prompt && rows[0].Response == ""
	}, 10*time.Second, 10*time.Millisecond)
}
