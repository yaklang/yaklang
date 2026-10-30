package aireact

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type longTrajectoryReceipt struct{ prompt, response string }

// The transport is deterministic; assembly, automatic overflow compression,
// action execution, task finalization and trajectory persistence are real.
func TestReActTrajectoryLongTaskPreservesEveryCompressionAndTerminal(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, cancelled := range []bool{false, true} {
			native, cancelled := native, cancelled
			t.Run(fmt.Sprintf("native=%t/cancel=%t", native, cancelled), func(t *testing.T) {
				t.Parallel()
				const waves, stepsPerWave = 4, 12
				ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
				defer cancel()
				sessionID := "trajectory-long-" + uuid.NewString()
				var runtime *ReAct
				var receiptMu sync.Mutex
				var receipts []longTrajectoryReceipt
				var decisions, compressions atomic.Int32
				var finalPrompt atomic.Pointer[string]
				stopRequested := make(chan struct{}, 1)
				var sources [waves]string
				for wave := range sources {
					var b strings.Builder
					fmt.Fprintf(&b, "WAVE_%d_BEGIN\n", wave)
					for row := 0; row < 6000; row++ {
						fmt.Fprintf(&b, "wave=%d row=%05d verified observation payload=%s\n", wave, row, strings.Repeat("original-data-", 4))
					}
					fmt.Fprintf(&b, "WAVE_%d_END", wave)
					sources[wave] = b.String()
				}
				terminalTail := "FINAL_CONTEXT_BEGIN\n" + strings.Repeat("Final independently verified observation preserves original data.\n", 3600) + "\nFINAL_CONTEXT_END"
				respond := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, name string, params aitool.InvokeParams) (*aicommon.AIResponse, string, error) {
					raw, err := json.Marshal(params)
					if err != nil {
						return nil, "", err
					}
					response := c.NewAIResponse()
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					provenance := map[string]any{"content": "", "reasoning_content": "", "tool_calls": []any{}, "finish_reason": "stop"}
					if native {
						id := fmt.Sprintf("long-%s-%d-%d", name, decisions.Load(), compressions.Load())
						wire.ToolCallCallback([]*aispec.ToolCall{{ID: id, Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
						wire.FinishReasonCallback("tool_calls", nil)
						provenance["finish_reason"] = "tool_calls"
						provenance["tool_calls"] = []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(raw)}}}
					} else {
						textParams := make(aitool.InvokeParams, len(params)+1)
						for k, v := range params {
							textParams[k] = v
						}
						textParams["@action"] = name
						raw, err = json.Marshal(textParams)
						if err != nil {
							return nil, "", err
						}
						finish := func() { wire.FinishReasonCallback("stop", nil) }
						if name == aicommon.ContextCompressionAction && compressions.Load() == 1 && !cancelled {
							// Provider completion may arrive well after a valid action.
							// It must not block the task or expire the trajectory receipt.
							time.AfterFunc(4*time.Second, finish)
						} else {
							finish()
						}
						response.EmitOutputStream(strings.NewReader(string(raw)))
						provenance["content"] = string(raw)
					}
					response.Close()
					encoded, err := json.Marshal(provenance)
					return response, string(encoded), err
				}
				var err error
				runtime, err = NewTestReAct(aicommon.WithContext(ctx), aicommon.WithSingleAIModelMode(true),
					aicommon.WithEnableFunctionCallMode(native), aicommon.WithAgreeYOLO(), aicommon.WithNoOpMemoryTriage(),
					aicommon.WithWorkdir(t.TempDir()), aicommon.WithPersistentSessionId(sessionID),
					aicommon.WithTimelineContentLimit(96*1024), aicommon.WithMaxIterationCount(80),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithEventHandler(func(*schema.AiOutputEvent) {}),
					aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
							return guiRespond(c, req, false, guiReplayStep{"verify-satisfaction", aitool.InvokeParams{"user_satisfied": true, "reasoning": "synthetic long task verified"}})
						}
						if strings.Contains(req.GetCallerLabel(), "directly-answer") {
							rsp := c.NewAIResponse()
							rsp.EmitOutputStream(strings.NewReader("All synthetic long-task observations were verified."))
							rsp.Close()
							return rsp, nil
						}
						if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
							wave := int(compressions.Add(1)) - 1
							if wave >= waves {
								return nil, fmt.Errorf("unexpected extra compression %d", wave)
							}
							if !strings.Contains(req.GetPrompt(), sources[wave]) {
								return nil, fmt.Errorf("wave %d was already truncated before compression", wave)
							}
							summary := fmt.Sprintf("COMPRESSED_WAVE_%d_BEGIN\n", wave) + strings.Repeat("verified summary fact; ", 2048) + fmt.Sprintf("\nCOMPRESSED_WAVE_%d_END", wave)
							rsp, raw, err := respond(c, req, aicommon.ContextCompressionAction, aitool.InvokeParams{"summary": summary, "retain_timeline": "", "retire_skill_names": []string{}, "retire_evidence_ids": []string{}})
							receiptMu.Lock()
							receipts = append(receipts, longTrajectoryReceipt{req.GetPrompt(), raw})
							receiptMu.Unlock()
							return rsp, err
						}
						step := int(decisions.Add(1))
						if step <= waves*stepsPerWave {
							if step%stepsPerWave == 0 {
								wave := step / stepsPerWave
								if wave < waves {
									runtime.config.Timeline.PushText(runtime.config.AcquireId(), "%s", sources[wave])
								} else {
									runtime.config.Timeline.PushText(runtime.config.AcquireId(), "%s", terminalTail)
								}
							}
							rsp, _, err := respond(c, req, "save_evidence", aitool.InvokeParams{"evidence_id": fmt.Sprintf("long.step.%d", step), "evidence_content": fmt.Sprintf("Synthetic step %d independently verified its local observation.", step)})
							return rsp, err
						}
						if cancelled {
							prompt := req.GetPrompt()
							finalPrompt.Store(&prompt)
							stopRequested <- struct{}{}
							<-req.GetContext().Done()
							return nil, req.GetContext().Err()
						}
						if step == waves*stepsPerWave+1 {
							rsp, _, err := respond(c, req, "directly_answer", aitool.InvokeParams{"answer_payload": "All synthetic long-task observations were verified."})
							return rsp, err
						}
						prompt := req.GetPrompt()
						finalPrompt.Store(&prompt)
						stopRequested <- struct{}{}
						rsp, _, err := respond(c, req, "finish", aitool.InvokeParams{})
						return rsp, err
					}))
				require.NoError(t, err)
				defer func() {
					cancel()
					runtime.WaitLifecycleStopped()
					_, _, _ = yakit.DeleteAISession(runtime.config.GetDB(), sessionID)
				}()
				runtime.config.Timeline.SetTimelineBucketByteSize(-1)
				runtime.config.Timeline.PushText(runtime.config.AcquireId(), "%s", sources[0])
				require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Verify the synthetic long task through all observation waves, then answer and finish."}))
				select {
				case <-stopRequested:
				case <-ctx.Done():
					t.Fatalf("long task did not reach its terminal request: decisions=%d compressions=%d", decisions.Load(), compressions.Load())
				}
				if cancelled {
					task := runtime.getProcessingRuntimeTask()
					require.NotNil(t, task)
					runtime.CancelTask(task, &ypb.AIInputEvent{})
				}
				status := "completed"
				if cancelled {
					status = "skipped"
				}
				var rows []*schema.AITrajectory
				require.Eventually(t, func() bool {
					rows, err = yakit.QueryAITrajectories(runtime.config.GetDB(), sessionID)
					return err == nil && len(rows) == waves+1 && rows[len(rows)-1].Kind == schema.AITrajectoryTerminal && rows[len(rows)-1].Status == status
				}, 10*time.Second, 10*time.Millisecond)
				receiptMu.Lock()
				defer receiptMu.Unlock()
				require.Len(t, receipts, waves)
				for i, expected := range receipts {
					require.Greater(t, len(expected.prompt), 400*1024)
					require.Equal(t, schema.AITrajectoryCompression, rows[i].Kind)
					require.Equal(t, len(expected.prompt), len(rows[i].Prompt))
					require.Equal(t, sha256.Sum256([]byte(expected.prompt)), sha256.Sum256([]byte(rows[i].Prompt)))
					require.Equal(t, sha256.Sum256([]byte(expected.response)), sha256.Sum256([]byte(rows[i].Response)))
					t.Logf("long trajectory compression=%d prompt_bytes=%d response_bytes=%d exact_match=true", i+1, len(rows[i].Prompt), len(rows[i].Response))
				}
				terminal := rows[len(rows)-1]
				expectedTerminal := finalPrompt.Load()
				require.NotNil(t, expectedTerminal)
				require.Greater(t, len(terminal.Prompt), 180*1024)
				require.Contains(t, terminal.Prompt, terminalTail)
				require.Equal(t, sha256.Sum256([]byte(*expectedTerminal)), sha256.Sum256([]byte(terminal.Prompt)))
				require.Empty(t, terminal.Response)
				t.Logf("long trajectory terminal status=%s decisions=%d prompt_bytes=%d compressions=%d total_rows=%d exact_match=true", status, decisions.Load(), len(terminal.Prompt), compressions.Load(), len(rows))
			})
		}
	}
}
