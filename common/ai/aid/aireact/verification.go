package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// VerifyUserSatisfaction verifies if the materials satisfied the user's needs and provides human-readable output
func (r *ReAct) VerifyUserSatisfaction(ctx context.Context, originalQuery string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}
	// Check context cancellation early
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	verificationPrompt, nonce, err := r.promptManager.GenerateVerificationPrompt(
		originalQuery, isToolCall, payload, r.DumpCurrentEnhanceData(),
	)
	if err != nil {
		return nil, utils.Errorf("generate verification prompt failed: %v", err)
	}
	if r.config.DebugPrompt {
		log.Infof("Verification prompt: %s", verificationPrompt)
	}

	result := &aicommon.VerifySatisfactionResult{}
	var referenceAnchorOnce sync.Once
	var referenceAnchorID string

	captureReferenceAnchor := func(event *schema.AiOutputEvent) {
		if event == nil {
			return
		}
		streamID := event.GetStreamEventWriterId()
		if streamID == "" {
			log.Errorf("empty streamId provided for verification reference anchor, origin data: %v", string(event.Content))
			return
		}
		referenceAnchorOnce.Do(func() {
			referenceAnchorID = streamID
		})
	}

	emitVerificationReferenceMaterials := func(emitter *aicommon.Emitter, rawResponse string) {
		if emitter == nil {
			return
		}
		if strings.TrimSpace(referenceAnchorID) == "" {
			log.Warnf("skip verification reference materials because no stream anchor was emitted")
			return
		}
		aicommon.EmitAIRequestAndResponseReferenceMaterials(emitter, referenceAnchorID, verificationPrompt, rawResponse)
	}

	log.Infof("Verifying if user needs are satisfied and formatting results...")
	// 同步 AI 调用 post-action 卡死兜底: 给 verification 的 AI 输出流套一层
	// StreamIdleTimeoutReader (TTFB / 字节间 idle 双阈值, 由 feature flag
	// EnableAIStreamIdleTimeout 控制, 默认开). 即便 feature flag 关闭,
	// reader 仍以 ttfb=0/idle=0 模式落地, 只做计时观测 (P0 埋点).
	// 关键词: VerifyUserSatisfaction 流空闲超时, P0 埋点, P1 兜底
	verifyTTFB, verifyIdle := aicommon.ResolveAIStreamIdleThresholds(r.config)
	var verificationTimedOut atomic.Bool

	transErr := aicommon.CallAITransaction(
		r.config, verificationPrompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			// 每个 retry attempt 独立重置, 仅当"最后一次失败"时才作为
			// 整体降级依据 (transErr != nil 时再读 verificationTimedOut).
			verificationTimedOut.Store(false)
			boundEmitter := rsp.BindEmitter(r.Emitter)
			rawStream := rsp.GetOutputStreamReader("re-act-verify", true, r.Emitter)

			idleReader := aicommon.NewStreamIdleTimeoutReader(rawStream, verifyTTFB, verifyIdle)
			defer func() {
				snap := idleReader.Snapshot()
				aicommon.LogStreamTimingSnapshot("VERIFY_AI_TIMING", snap)
				if snap.TimedOut {
					verificationTimedOut.Store(true)
				}
				_ = idleReader.Close()
			}()
			stream := io.Reader(idleReader)

			var rawResponse bytes.Buffer
			stream = io.TeeReader(stream, &rawResponse)

			createReasonCallback := func(prompt string) func(key string, reader io.Reader) {
				return func(key string, reader io.Reader) {
					var out bytes.Buffer
					reader = io.TeeReader(utils.JSONStringReader(utils.UTF8Reader(reader)), &out)
					var event *schema.AiOutputEvent
					var err error
					event, err = boundEmitter.EmitDefaultSystemStreamEvent(
						"re-act-verify",
						reader,
						rsp.GetTaskIndex(),
						func() {
							if out.Len() > 0 {
								r.AddToTimeline("verify", prompt+": "+out.String())
							}
						},
					)
					if err != nil {
						log.Errorf("failed to emit %s stream event: %v", key, err)
						return
					}
					captureReferenceAnchor(event)
				}
			}

			action, err := aicommon.ExtractValidActionFromStream(
				ctx,
				stream, "verify-satisfaction",
				aicommon.WithActionNonce(nonce),
				aicommon.WithActionTagToKey("EVIDENCE", "evidence"),
				aicommon.WithActionFieldStreamHandler(
					[]string{"reasoning"},
					createReasonCallback("Reasoning"),
				),
				aicommon.WithActionFieldStreamHandler(
					[]string{"evidence"},
					func(key string, rd io.Reader) {
						trimmedReader := utils.NewTrimLeftReader(utils.UTF8Reader(rd))
						peekedReader := utils.NewPeekableReader(trimmedReader)
						firstByte, err := peekedReader.Peek(1)
						if err != nil && len(firstByte) == 0 {
							log.Infof("no evidence provided in verification result, skipping evidence stream handling")
							return
						}

						var displayReader io.Reader
						if len(firstByte) > 0 && firstByte[0] == '[' {
							pr, pw := utils.NewBufPipe(nil)
							go func() {
								defer pw.Close()
								if err := writeEvidenceDisplayStream(peekedReader, pw); err != nil {
									log.Errorf("failed to stream evidence display: %v", err)
								}
							}()
							displayReader = pr
						} else {
							var buf bytes.Buffer
							io.Copy(&buf, peekedReader)
							content := strings.TrimSpace(buf.String())
							if content == "" {
								log.Infof("evidence content is empty after trim, skipping emit")
								return
							}
							formatted := formatEvidenceOperationDisplayLine(aicommon.EvidenceOperation{
								Op: "add", ID: "default", Content: content,
							})
							if strings.TrimSpace(formatted) == "" {
								return
							}
							displayReader = strings.NewReader(formatted)
						}

						var out bytes.Buffer
						var outputReader = io.TeeReader(displayReader, &out)
						var event *schema.AiOutputEvent
						event, err = boundEmitter.EmitDefaultSystemStreamEvent(
							"plan-evidence",
							outputReader,
							rsp.GetTaskIndex(),
							func() {},
						)
						if err != nil {
							log.Errorf("failed to emit evidence stream event: %v", err)
							return
						}
						captureReferenceAnchor(event)
					},
				),
			)
			if err != nil {
				return utils.Errorf("failed to extract verification action: %v, need ...\"@action\":\"verify-satisfaction\" ", err)
			}
			// If we found a proper @action structure, extract data from it
			result.Satisfied = action.GetBool("user_satisfied")
			result.Reasoning = action.GetString("reasoning")
			result.CompletedTaskIndex = action.GetString("completed_task_index")
			result.Evidence = strings.TrimSpace(action.GetString("evidence"))
			result.EvidenceOps = normalizeEvidenceOperations(action)

			if len(result.EvidenceOps) > 0 {
				var opSummary []string
				for _, eop := range result.EvidenceOps {
					opSummary = append(opSummary, fmt.Sprintf("%s[%s]", strings.ToUpper(eop.Op), eop.ID))
				}
				r.AddToTimeline("evidence_ops", strings.Join(opSummary, "; "))
			}

			// verification 不再产出 next_movements: TODO 维护职责完全交给
			// 主循环的 adjust_todolist action 和 next_movements 兜底入口.
			// verification 收缩为纯观测角色, 只产出 evidence + satisfied/
			// reasoning/completed_task_index 作为观测信号.
			// 关键词: verification 纯观测, 不写 next_movements, 不维护 TODO,
			// adjust_todolist 是 TODO 权威入口

			emitVerificationReferenceMaterials(boundEmitter, rawResponse.String())
			return nil
		},
		aicommon.WithAIRequest_CallerLabel("verification"),
	)
	if transErr != nil {
		log.Errorf("AI transaction failed during verification: %v", transErr)
		// P1 兜底: 当所有 retry 都因 AI 流"假活"(无字节/字节间空闲)而失败时,
		// 不再向上抛出 transErr 让调用方 Fail loop, 而是降级为
		// "本轮 verification 跳过 = 视为未满足", 同时 timeline 留下
		// [VERIFICATION_TIMEOUT] 痕迹供前端 / 后续 prompt / 自我反思感知.
		// 关键词: VerifyUserSatisfaction 流空闲降级, [VERIFICATION_TIMEOUT]
		if verificationTimedOut.Load() {
			r.AddToTimeline("[VERIFICATION_TIMEOUT]", fmt.Sprintf(
				"AI verification stream idle timeout (ttfb=%v idle=%v); skipped this round, treated as not-satisfied so the loop keeps moving",
				verifyTTFB, verifyIdle,
			))
			skipped := &aicommon.VerifySatisfactionResult{
				Satisfied: false,
				Reasoning: fmt.Sprintf(
					"Verification skipped due to AI stream idle timeout (ttfb=%v idle=%v); treating as not-satisfied so the loop continues without blocking",
					verifyTTFB, verifyIdle,
				),
			}
			return skipped, nil
		}
		return nil, transErr
	}
	// verification 不再写 TODO store (AppendVerificationHistory 已移除):
	// satisfied 作为观测信号沉淀到 satisfaction record (由调用方
	// PushSatisfactionRecord* 完成), Evidence 由调用方 ApplySessionEvidenceOps
	// 写入. enforceTodoCompletionBeforeSatisfaction 第二道仍保留, 用 store 残留
	// TODO 作为客观门推翻 AI 的 satisfied=true 主观声明 (store 由
	// adjust_todolist / 主循环兜底维护).
	// 关键词: verification 不写 store, satisfied 客观门第二道保留
	r.enforceTodoCompletionBeforeSatisfaction(result)

	return result, nil
}

// enforceTodoCompletionBeforeSatisfaction is the Satisfied bottom-line override.
//
// 控制论视角: AI 输出的 user_satisfied=true 是控制器的"已达稳态"信号. 但
// 当全局 TODO store 还有 PENDING/DOING 项, 说明可能性空间内仍存在 AI 自己
// 列出的待完成动作, 这与"已达稳态"在控制语义上互相冲突. 此时我们用
// SessionPromptState 中可观测的 TODO 状态作为客观反馈, 推翻 AI 的主观
// 声明 — 把 user_satisfied 强制回退为 false, 并写一条
// [VERIFICATION_TODO_INCOMPLETE] timeline 把"残留 TODO"作为下一轮的输入
// 反馈给 AI, 形成闭环.
//
// 触发条件:
//  1. result.Satisfied == true
//  2. SessionPromptState 中 stats.Pending + stats.Doing > 0
//
// 副作用:
//   - result.Satisfied 翻为 false
//   - result.Reasoning 前缀注入 [OVERRIDE]，保留 AI 原文于 [AI ORIGINAL]
//   - timeline 写入 [VERIFICATION_TODO_INCOMPLETE], 列出残留 TODO 摘要
//
// 注: 原第一道 (检查 result.NextMovements 里有没有 add op) 已移除 ——
// verification 不再产出 next_movements, 这道永远不触发. TODO store 的写入
// 现在由 adjust_todolist action 和主循环 next_movements 兜底入口承担, 本
// 函数只做第二道: 读 store 残留 TODO 作为客观门.
//
// 关键词: enforceTodoCompletionBeforeSatisfaction, Satisfied 兜底回退,
//
//	[VERIFICATION_TODO_INCOMPLETE], 闭环反馈, 客观 TODO 反馈推翻主观声明
func (r *ReAct) enforceTodoCompletionBeforeSatisfaction(result *aicommon.VerifySatisfactionResult) {
	if r == nil || result == nil {
		return
	}
	if !result.Satisfied {
		return
	}
	if r.config == nil {
		return
	}

	currentTask := r.GetCurrentTask()
	scope := aicommon.BuildVerificationTodoScope(currentTask)
	if scope.IsZero() {
		log.Infof("enforceTodoCompletion: scope is zero (no task id), skipping store-based check")
		return
	}

	items := r.config.ActiveVerificationTodoItemsByScope(scope)
	activeTotal := len(items)
	if activeTotal == 0 {
		return
	}
	stats := r.config.GetVerificationTodoStatsByScope(scope)
	activeLines := make([]string, 0, activeTotal)
	for _, item := range items {
		activeLines = append(activeLines, aicommon.FormatVerificationTodoLine(item))
	}

	msg := fmt.Sprintf(
		"AI declared user_satisfied=true but %d active TODO item(s) still remain (pending=%d, doing=%d). "+
			"user_satisfied has been force-overridden to false. Each remaining TODO must be explicitly closed "+
			"via next_movements with op=done / op=delete / op=skip before completion can be acknowledged. "+
			"Remaining TODOs:\n%s",
		activeTotal, stats.Pending, stats.Doing, strings.Join(activeLines, "\n"),
	)

	result.Satisfied = false
	originalReasoning := strings.TrimSpace(result.Reasoning)
	if originalReasoning == "" {
		result.Reasoning = "[OVERRIDE] " + msg
	} else {
		result.Reasoning = "[OVERRIDE] " + msg + "\n\n[AI ORIGINAL] " + originalReasoning
	}

	r.AddToTimeline("[VERIFICATION_TODO_INCOMPLETE]", msg)
	log.Warnf("verification satisfied override: %d active TODO(s) remain, forcing user_satisfied=false", activeTotal)
}

// addNextMovementsBreadcrumb / emitTodoListUpdate 已随 verification 不再产出
// next_movements 一并移除. verification 收缩为纯观测角色后:
//   - 不再写 NEXT_MOVEMENTS timeline breadcrumb (TODO 增量由 adjust_todolist
//     主循环通道自行写 timeline);
//   - 不再发 EVENT_TYPE_TODO_LIST_UPDATE (前端 TODO 面板的权威更新源改为
//     adjust_todolist / 主循环 next_movements 兜底入口).
//
// 关键词: verification 不写 next_movements breadcrumb, 不发 TodoListUpdate

// writeNextMovementsDisplayStream / formatNextMovementDisplayLine 都已经
// 抽到 aicommon 包成为公开 helper, 这里保留 package-local 薄包装是为了:
//  1. 让 verification_compat_test.go 等历史调用点继续按原符号引用, 无需大改;
//  2. 让 adjust_todolist 主循环路径与 verification 共享同一份字节流转换,
//     避免双通道字符级漂移.
//
// 关键词: writeNextMovementsDisplayStream 兼容层, formatNextMovementDisplayLine
//
//	薄包装, aicommon 单源, verification + adjust_todolist 双通道一致
func writeNextMovementsDisplayStream(reader io.Reader, writer io.Writer) error {
	return aicommon.WriteNextMovementsDisplayStream(reader, writer)
}

func formatNextMovementDisplayLine(movement aicommon.VerifyNextMovement) string {
	return aicommon.FormatNextMovementDisplayLine(movement)
}

func writeEvidenceDisplayStream(reader io.Reader, writer io.Writer) error {
	decoder := json.NewDecoder(reader)
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '[' {
		return utils.Errorf("evidence is not a JSON array")
	}

	firstLine := true
	for decoder.More() {
		var op aicommon.EvidenceOperation
		if err := decoder.Decode(&op); err != nil {
			return err
		}
		line := formatEvidenceOperationDisplayLine(op)
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !firstLine {
			if _, err := writer.Write([]byte("\n")); err != nil {
				return err
			}
		}
		firstLine = false
		if _, err := io.WriteString(writer, line); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func formatEvidenceOperationDisplayLine(op aicommon.EvidenceOperation) string {
	return aicommon.FormatEvidenceOpLine(op, "")
}

func normalizeEvidenceOperations(action *aicommon.Action) []aicommon.EvidenceOperation {
	if action == nil {
		return nil
	}
	evidenceArray := action.GetInvokeParamsArray("evidence")
	if len(evidenceArray) == 0 {
		legacy := strings.TrimSpace(action.GetString("evidence"))
		if legacy == "" {
			return nil
		}
		return []aicommon.EvidenceOperation{{
			ID:      "default",
			Op:      "add",
			Content: legacy,
		}}
	}
	ops := make([]aicommon.EvidenceOperation, 0, len(evidenceArray))
	for _, item := range evidenceArray {
		if item == nil {
			continue
		}
		op := strings.ToLower(strings.TrimSpace(item.GetString("op")))
		id := strings.TrimSpace(item.GetString("id"))
		content := strings.TrimSpace(item.GetString("content"))
		if op == "" || id == "" {
			continue
		}
		ops = append(ops, aicommon.EvidenceOperation{
			ID:      id,
			Op:      op,
			Content: content,
		})
	}
	return ops
}

// normalizeVerifyNextMovements is a thin wrapper around the public
// aicommon.NormalizeVerifyNextMovements helper. The verification path keeps
// its own private symbol so existing call sites and tests in this package
// do not need to be touched, while the underlying parsing logic stays in
// aicommon and is also reused by the main-loop adjust_todolist action.
//
// 关键词: normalizeVerifyNextMovements thin wrapper, aicommon 单源,
//
//	adjust_todolist 复用
func normalizeVerifyNextMovements(action *aicommon.Action) []aicommon.VerifyNextMovement {
	return aicommon.NormalizeVerifyNextMovements(action)
}
