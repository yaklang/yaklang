package reactloops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// synchronizedResponseCapture is a bytes.Buffer with snapshot reads. Action
// parsing is asynchronous after ExtractActionFromStream returns: the mirror
// goroutine may still be driving the tee while the transaction post-handler is
// validating fields. Keeping synchronization at the diagnostic capture avoids
// racing buf.String against Write without changing Action's per-field streaming
// API.
type synchronizedResponseCapture struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (c *synchronizedResponseCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *synchronizedResponseCapture) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buf.String()
}

// isJSONEmbeddedAITagPrefix 判断字段流的首批 peek 字节是否以 `<|TagName_` 开头,
// 兼容 JSON 字段流推过来的 raw bytes 通常带外层 `"` 与零宽空白. 命中即视为 AI 把
// AITag 块塞进了 JSON 字符串值, 触发 JSON / AITag 双 emit 重复, 让调用方静默
// drain 该路径以让 AITag 流单独负责干净 emit.
//
// 关键词: JSON-embedded AITag prefix detect, peek 跳过 quote/whitespace, 字段流去重
func isJSONEmbeddedAITagPrefix(peeked []byte, wrapperToken string) bool {
	if len(peeked) == 0 || wrapperToken == "" {
		return false
	}
	// 跳过 JSON 字符串外层可能的 leading `"` 与若干空白 (包含全角 BOM 安全裕度).
	i := 0
	for i < len(peeked) {
		switch peeked[i] {
		case '"', ' ', '\t', '\r', '\n':
			i++
			continue
		}
		break
	}
	return bytes.HasPrefix(peeked[i:], []byte(wrapperToken))
}

// waitReadableStream blocks until the stream yields at least one byte or closes.
// It lets callers avoid creating frontend stream cards for empty streams while
// still preserving the first byte for later emit.
func waitReadableStream(reader io.Reader) (*utils.BufferedUTF8PeekableReader, bool, error) {
	peekedReader := utils.NewUTF8PeekableReader(reader)
	firstByte, err := peekedReader.Peek(1)
	if err != nil && len(firstByte) == 0 {
		if errors.Is(err, io.EOF) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return peekedReader, true, nil
}

func (r *ReActLoop) buildActionTagOption(emitter *aicommon.Emitter, streamWG *sync.WaitGroup, taskIndex string, nonce string) []aicommon.ActionMakerOption {
	tagFields := r.aiTagFields.Copy()
	for _, i := range r.GetAllActions() {
		for _, field := range i.AITagStreamFields {
			tagFields.Set(field.TagName, field)
		}
	}
	var actionOptions []aicommon.ActionMakerOption
	actionOptions = append(actionOptions, aicommon.WithActionNonce(nonce))

	waitStream := utils.NewOnce()

	for _, _tagInstance := range tagFields.Values() {
		waitStream.Do(func() {
			streamWG.Add(1)
			actionOptions = append(actionOptions, aicommon.WithActionOnReaderFinished(func() {
				streamWG.Done()
			}))
		})

		v := _tagInstance

		// 字段级双注册: 默认走 turn nonce; 同时无条件追加
		// aicommon.LiteralCurrentNoncePlaceholder ("CURRENT_NONCE") 作为兜底
		// nonce 候选. 如果 LoopAITagField.ExtraNonces 还显式声明了其他候选
		// (例如 [current-nonce]), 也会一并注册.
		//
		// 兜底 CURRENT_NONCE 的原因: 各 loop 的 persistent_instruction /
		// output_example 等示例 prompt 普遍写成 `<|FACTS_CURRENT_NONCE|>` /
		// `<|FINAL_ANSWER_CURRENT_NONCE|>` 等占位符形式, 设计本意是让 AI 替换
		// 为本 turn 实际 nonce. 但实测部分模型会把 CURRENT_NONCE 当作字面量
		// 直接照抄输出, 此时只用 turn nonce 注册 callback 会匹配失败, 内容
		// 丢失, 触发 verifier 5 次重试黑洞 (典型: output_facts: facts content
		// is required). 双注册让两种输出格式都能命中.
		//
		// 用例: CACHE_TOOL_CALL 块内 TOOL_PARAM_xxx 在 prompt 中用占位符字面量
		// nonce "[current-nonce]" 渲染保持字节稳定; LLM 既可能照抄字面量,
		// 也可能识破替换为 turn nonce. 显式 ExtraNonces 兼容这种行为.
		//
		// 关键词: buildActionTagOption, ExtraNonces 双注册, CURRENT_NONCE 兜底,
		//
		//	AI 占位符照抄, [current-nonce]
		extraNonceCandidates := []string{aicommon.LiteralCurrentNoncePlaceholder}
		extraNonceCandidates = append(extraNonceCandidates, v.ExtraNonces...)
		actionOptions = append(actionOptions,
			aicommon.WithActionTagToKeyAndExtraNonces(v.TagName, v.VariableName, extraNonceCandidates...),
		)
		actionOptions = append(actionOptions,
			aicommon.WithActionFieldStreamHandler([]string{v.VariableName}, func(key string, fieldReader io.Reader) {
				nodeId := v.AINodeId
				contentType := v.ContentType
				if nodeId == "" {
					nodeId = "re-act-loop-answer-payload"
				}

				if contentType == "" {
					contentType = "text/plain"
				}

				// check empty tag
				peekedReader, readable, err := waitReadableStream(fieldReader)
				if err != nil {
					log.Warnf("field stream handler[%s]: failed waiting first byte before emit: %v", v.TagName, err)
					r.Set(v.VariableName, "")
					return
				}
				if !readable {
					log.Debugf("field stream handler[%s]: stream closed before first byte, skipping empty emit", v.TagName)
					r.Set(v.VariableName, "")
					return
				}

				// JSON-embedded AITag wrapper de-dup:
				// 同一个字段 (例如 facts) 会被 ActionMaker 同时通过 JSON 字段流和 AITag
				// 流两条路径推到当前 handler 里, 各 emit 一次, 导致前端"事实"事件重复.
				//
				// 实测中, 如果 AI 把 `<|FACTS_<nonce>|>...<|FACTS_END_<nonce>|>` 整段
				// 塞进 JSON `facts` 字符串值 (不论是把 wrappers 当字面量包进去, 还是
				// 同时又在 JSON 外再写一遍 AITag 块), JSON 路径会带着 wrappers 推到这
				// 里, 而 AITag 路径会另起一路推干净的内层. 用户看到一条带 `<|...|>` 字
				// 面量+反斜杠 n 的丑文本, 一条干净的 markdown, 极差体验.
				//
				// 修法: peek 流首批字节, 若 (跳过 JSON token 边界字符如外层 `"` 与
				// 空白后) 以本 tag 的起始 token `<|TagName_` 开头, 判定这是 JSON 路径
				// 误报的重复, 静默 drain 不再 emit, 让 AITag 路径专心输出干净版本.
				// 注意 JSON 字段流推过来的 raw bytes 通常包含外层引号 (例如
				// `"<|FACTS_...<|FACTS_END_..."`), 这是和 AITag 路径推过来的纯内层
				// 内容的关键区分点; 不能只匹配 `<|TagName_` 而要兼容前置 `"` /
				// whitespace.
				//
				// 关键词: 字段流去重, JSON-embedded AITag, FACTS 重复 emit 修复,
				//        peek 检测 <|TagName_ 前缀, 兼容 JSON 外层引号, drain 静默丢弃
				wrapperToken := "<|" + v.TagName + "_"
				const peekWindow = 32
				peeked, _ := peekedReader.Peek(peekWindow)
				if isJSONEmbeddedAITagPrefix(peeked, wrapperToken) {
					drained, _ := io.Copy(io.Discard, peekedReader)
					log.Debugf("field stream handler[%s]: detected JSON-embedded AITag wrapper "+
						"(token %q in peek %q), dropped duplicate stream (%d bytes drained); "+
						"AITag stream path will emit the clean inner content",
						v.TagName, wrapperToken, string(peeked), drained)
					return
				}

				callbackStart := time.Now()
				var result bytes.Buffer
				teedReader := io.TeeReader(peekedReader, &result)
				wg := sync.WaitGroup{}
				wg.Add(1)
				_, eventErr := emitter.EmitStreamEventWithContentType(
					nodeId, teedReader, taskIndex, contentType,
					func() {
						defer wg.Done()
						// Use parseStart instead of callbackStart to measure the whole streaming process
						r.Set(v.VariableName, result.String())
						totalCost := time.Since(callbackStart)
						contentLength := len(result.String())
						log.Debugf("tag[%s] callback finished, content length: %d chars, total stream cost: %v",
							v.TagName, contentLength, totalCost)

						// Buffered responses can drain immediately; latency is not a
						// validity check. Parsing and action validation report failures.
					},
				)
				if eventErr != nil {
					wg.Done()
					r.Set(v.VariableName, result.String())
					log.Errorf("tag[%s] EmitStreamEventWithContentType failed: %v", v.TagName, eventErr)
					return
				}
				wg.Wait()
			}),
		)
	}
	return actionOptions
}

func inferActionTypeFromPayload(action *aicommon.Action, finalAnswer string) string {
	if action == nil {
		return ""
	}

	if candidate := strings.TrimSpace(action.ActionType()); candidate != "" && candidate != "object" {
		return candidate
	}
	// GetString also exposes flattened nested keys for legacy field consumers.
	// A tool parameter such as params.type="A" is not an action discriminator.
	params := action.GetParams()
	nextAction := params.GetObject("next_action")
	candidates := []string{
		strings.TrimSpace(nextAction.GetString("type")),
		strings.TrimSpace(params.GetString("type")),
	}
	for _, candidate := range candidates {
		if candidate != "" && candidate != "object" {
			return candidate
		}
	}

	hasField := func(key string) bool {
		if strings.TrimSpace(params.GetString(key)) != "" {
			return true
		}
		if strings.TrimSpace(nextAction.GetString(key)) != "" {
			return true
		}
		return false
	}
	hasCanonicalField := func(key string) bool {
		if _, ok := params[key]; ok {
			return true
		}
		if nextAction != nil {
			_, ok := nextAction[key]
			return ok
		}
		return false
	}

	if strings.TrimSpace(finalAnswer) != "" || hasField("answer_payload") {
		return "directly_answer"
	}
	if hasField("tool_require_payload") || hasCanonicalField("tool_require_calls") {
		return "require_tool"
	}
	if hasField("directly_call_tool_name") || hasField("directly_call_identifier") || hasCanonicalField("directly_call_tool_calls") {
		return "directly_call_tool"
	}
	if hasField("tool_compose_payload") {
		return "tool_compose"
	}
	if hasField("capability_identifier") {
		return "load_capability"
	}
	if hasField("rewrite_user_query_for_knowledge_enhance") {
		return "knowledge_enhance_answer"
	}
	if hasField("blueprint_payload") {
		return "require_ai_blueprint"
	}
	if hasField("plan_request_payload") {
		return "request_plan_and_execution"
	}
	if hasField("skill_name") || hasField("skill_names") {
		return "loading_skills"
	}
	if hasField("resource_path") || hasField("pattern") {
		return "load_skill_resources"
	}
	return ""
}

func actionTypeResolutionError(requested string, availableActions []string, reason string) error {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "<missing>"
	}
	return utils.Errorf(
		"action resolution failed: requested=%q; matcher=exact registered action or alias; available_actions=%v; reason=%s",
		requested,
		availableActions,
		reason,
	)
}

func (r *ReActLoop) Execute(taskId string, ctx context.Context, userInput string) error {
	task := aicommon.NewStatefulTaskBase(
		taskId,
		userInput,
		ctx,
		r.GetEmitter(),
	)

	if r.onTaskCreated != nil {
		r.onTaskCreated(task)
	}

	utils.Debug(func() {
		fmt.Println("---------------------------------------------")
		fmt.Println("start to handle userInput \n" + utils.PrefixLines(userInput, "> "))
		fmt.Println("---------------------------------------------")
	})
	defer func() {
		utils.Debug(func() {
			fmt.Println("---------------------------------------------")
			fmt.Println("end to handle userInput \n" + utils.PrefixLines(userInput, "> "))
			fmt.Println("---------------------------------------------")
		})
	}()
	err := r.ExecuteWithExistedTask(task)
	if task.IsAsyncMode() {
		return err
	}
	task.Finish(err)
	return err
}

const ReActLoadingStatusKey = "re-act-loading-status-key"

func (r *ReActLoop) loadingStatus(i string) {
	if r.emitter == nil {
		return
	}
	log.Infof("re-act-loop loading status updated: %v", i)
	zh, en := aicommon.SplitLegacyStatusI18n(i)
	_, _ = r.emitter.EmitStatusI18n(ReActLoadingStatusKey, zh, en)
}

// UserStatus emits a product-facing status with a Chinese-compatible legacy
// value and optional structured metadata for newer clients.
func (r *ReActLoop) UserStatus(zh, en string, options ...aicommon.StatusOption) {
	if utils.IsNil(r) || r.emitter == nil {
		return
	}
	log.Infof("re-act-loop user status updated: %v", zh)
	_, _ = r.emitter.EmitStatusI18n(ReActLoadingStatusKey, zh, en, options...)
}

func (r *ReActLoop) LoadingStatus(i string) {
	if utils.IsNil(r) {
		return
	}
	r.loadingStatus(i)
}

func (r *ReActLoop) ExecuteWithExistedTask(task aicommon.AIStatefulTask) (finalError error) {
	r.UserStatus(
		"正在准备这次任务",
		"Preparing this task",
		aicommon.WithStatusCode("task.preparing"),
	)
	if !r.noEndLoadingStatus {
		defer func() {
			if finalError != nil {
				r.UserStatus(
					"这次任务已结束，但还有问题没有解决",
					"This task has ended with unresolved issues",
					aicommon.WithStatusCode("task.failed"),
					aicommon.WithStatusState(aicommon.StatusStateError),
				)
				return
			}
			r.UserStatus(
				"这次任务已经处理完成",
				"This task has been completed",
				aicommon.WithStatusCode("task.completed"),
				aicommon.WithStatusState(aicommon.StatusStateSuccess),
			)
		}()
	}
	defer r.Release()
	defer r.shutdownSubAgents()

	if utils.IsNil(task) {
		return errors.New("re-act loop task is nil")
	}

	if r == nil {
		return errors.New("re-act loop is nil")
	}
	if r.taskMutex == nil {
		return errors.New("re-act loop taskMutex is nil")
	}
	if taskEmitter := task.GetEmitter(); taskEmitter != nil {
		loopEmitter := r.emitter
		if loopEmitter != nil && loopEmitter != taskEmitter {
			r.emitter = taskEmitter.PushEventProcessersFrom(loopEmitter)
		} else {
			r.emitter = taskEmitter
		}
		defer func() {
			r.emitter = loopEmitter
		}()
	}

	select {
	case <-task.GetContext().Done():
		return utils.Errorf("task context done before execute ReActLoop: %v", task.GetContext().Err())
	default:
	}

	r.SetCurrentTask(task)
	r.ensureLoopDirectory(task)

	// Emit loop-enter lifecycle marker. This records the loop name, the task it
	// is executing on, and the parent task (if known) so that viz can reconstruct
	// the real execution stack without inferring relationships from task id strings.
	parentTaskID := ""
	if inv := r.GetInvoker(); inv != nil {
		if parentTask := inv.GetCurrentTask(); parentTask != nil && parentTask.GetId() != task.GetId() {
			parentTaskID = parentTask.GetId()
		}
	}
	if emitter := r.GetEmitter(); emitter != nil {
		_, _ = emitter.EmitStructured("loop_marker", map[string]any{
			"loop_kind":      "loop",
			"loop_name":      r.GetLoopName(),
			"task_id":        task.GetId(),
			"task_name":      task.GetName(),
			"parent_task_id": parentTaskID,
			"marker":         "enter",
		})
	}

	// Initialize action constraints from init handler
	var initOperator *InitTaskOperator

	if r.initHandler != nil {
		r.UserStatus(
			"正在了解任务背景",
			"Reviewing the task context",
			aicommon.WithStatusCode("task.context"),
		)
		utils.Debug(func() {
			fmt.Println("================================================")
			fmt.Printf("re-act loop [%v] task init handler start to execute\n", r.loopName)
			fmt.Println("================================================")
		})

		// Save current task before initHandler, as it may execute sub-loops
		// (e.g. intent recognition) that will call SetCurrentTask with different tasks
		savedTask := r.GetCurrentTask()
		initOperator = newInitTaskOperator()
		r.initHandler(r, task, initOperator)
		// Restore the original task after initHandler
		if savedTask != nil {
			r.SetCurrentTask(savedTask)
		}

		// Check operator status
		if initOperator.IsDone() {
			// Init handler completed the task, exit immediately (early routing)
			log.Infof("ReactLoop[%v] init handler signaled Done, exiting early", r.loopName)
			return nil
		}

		if failed, failErr := initOperator.IsFailed(); failed {
			r.UserStatus(
				"准备任务时遇到问题",
				"A problem occurred while preparing the task",
				aicommon.WithStatusCode("task.prepare_failed"),
				aicommon.WithStatusState(aicommon.StatusStateError),
			)
			inv := r.GetInvoker()
			inv.AddToTimeline("error", fmt.Sprintf("ReActLoop[%v] task init handler execute failed: %v", r.loopName, failErr))
			query := "Task initialization failed: " + failErr.Error() + "\n\n Origin INPUT: " + task.GetUserInput() + "\n\n Please give some practical advice for fix this issue or help user"
			ctx := inv.GetConfig().GetContext()
			if !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			result, err := inv.DirectlyAnswer(ctx, query, nil)
			if err != nil {
				return utils.Errorf("re-act loop [%v] task init handler execute failed: %v; additionally, failed to get direct answer: %v", r.loopName, failErr, err)
			}
			inv.EmitFileArtifactWithExt("init_error_advice.txt", ".md", result)
			return utils.Errorf("re-act loop [%v] task init handler execute failed: %v", r.loopName, failErr)
		}

		// Continue with normal execution
		// Apply action constraints from init handler
		if initOperator.HasActionConstraints() {
			r.initActionMustUse = initOperator.GetNextActionMustUse()
			r.initActionDisabled = initOperator.GetNextActionDisabled()
			r.initActionApplied = false // Will be applied in first iteration
			log.Infof("ReactLoop[%v] init set action constraints: must_use=%v, disabled=%v",
				r.loopName, r.initActionMustUse, r.initActionDisabled)
		}
	}

	// 工具 in-flight 必须始终挂到 config hook 上, 即使本 loop 关闭了
	// periodic verification: stall heartbeat 靠它判断"长 grep 还在跑",
	// 不能跟 verification watchdog 绑死.
	var clearWatchdogToolHooks func()
	if inv := r.GetInvoker(); inv != nil {
		if cfg, ok := inv.GetConfig().(*aicommon.Config); ok {
			cfg.SetVerificationWatchdogToolBlockingHooks(
				func() {
					r.BeginToolActivity()
					if !r.DisablePeriodicVerification {
						r.BeginVerificationWatchdogToolSuppression()
					}
				},
				func() {
					if !r.DisablePeriodicVerification {
						r.EndVerificationWatchdogToolSuppression()
					}
					r.EndToolActivity()
				},
			)
			clearWatchdogToolHooks = func() {
				cfg.SetVerificationWatchdogToolBlockingHooks(nil, nil)
			}
		}
	}
	if !r.DisablePeriodicVerification {
		r.startVerificationWatchdog(task)
	}
	defer func() {
		if clearWatchdogToolHooks != nil {
			clearWatchdogToolHooks()
		}
		if !r.DisablePeriodicVerification {
			r.stopVerificationWatchdogForTask(task) // 退出循环则停止验证看门狗，因为异步长任务不需要验证
		}
	}()

	done := utils.NewOnce()
	abort := func(err error) {
		r.shutdownSubAgents()
		done.Do(func() {
			// 用户通过 sync 事件主动取消/跳过时，不覆盖已设的终止状态，
			// 也不追加 [Error] 到 result，避免污染用户可见的取消语义。
			if task.IsUserCancelled() || testIsFinished(task) {
				return
			}
			result := task.GetResult()
			result += "\n\n[Error]: " + err.Error()
			task.SetResult(result)
			task.SetStatus(aicommon.AITaskState_Aborted)
		})
	}
	complete := func(err any) {
		r.shutdownSubAgents()
		if !utils.IsNil(err) {
			result := task.GetResult()
			result += "\n\n[Reason]: " + utils.InterfaceToString(err)
			task.SetResult(result)
		}
		done.Do(func() {
			if task.GetStatus() == aicommon.AITaskState_Skipped {
				log.Infof("re-act loop [%v] task[%v] skipped", r.loopName, task.GetId())
			} else {
				task.SetStatus(aicommon.AITaskState_Completed)
			}
		})
	}

	taskStartProcessing := func() {
		task.SetStatus(aicommon.AITaskState_Processing)
	}

	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
			finalError = utils.Errorf("ReActLoop panicked: %v", err)
		}
		// Recover before choosing a terminal state. A separate, later defer
		// would see a nil finalError during panic unwinding and commit Completed.
		if finalError != nil {
			abort(finalError)
		} else {
			complete(nil)
		}
	}()

	nonce := utils.RandStringBytes(4)
	_ = nonce

	var iterationCount int
	var maxIterations int
	if r.maxIterations > 0 {
		maxIterations = r.maxIterations
	} else {
		maxIterations = 100
	}
	var emitter = r.emitter
	if utils.IsNil(emitter) {
		abort(utils.Errorf("Emitter is nil"))
		return utils.Error("emitter is nil in ReActLoop")
	}

	if r.NoActions() {
		abort(utils.Errorf("no action names in ReActLoop"))
		return utils.Error("no action names in ReActLoop")
	}

	var operator = newLoopActionHandlerOperator(task)

	if task.GetStatus() == aicommon.AITaskState_Skipped {
		return utils.Errorf("ReActLoop task is skipped")
	}

	taskStartProcessing()

	// Initialize timeline differ to track changes during this task execution
	// This captures the baseline BEFORE any task-related timeline entries are added
	// We get the timeline from the invoker's config
	if invoker := r.GetInvoker(); invoker != nil {
		if cfg := invoker.GetConfig(); cfg != nil {
			if configWithTimeline, ok := cfg.(*aicommon.Config); ok && configWithTimeline.Timeline != nil {
				r.timelineDiffer = aicommon.NewTimelineDiffer(configWithTimeline.Timeline)
				r.timelineDiffer.SetBaseline()
				log.Debugf("ReactLoop[%s] timeline baseline set, items: %d", r.loopName, configWithTimeline.Timeline.GetIdToTimelineItem().Len())
			}
		}
	}

	r.GetInvoker().AddToTimeline(aicommon.TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT, fmt.Sprintf("%v", task.GetOriginUserInput()))

	// 启动主循环卡死兜底观察 goroutine: 周期性比对 lastIterationTickAt,
	// 长时间无推进就 emit timeline + dump goroutine stack. 不会主动 abort
	// 任务, 只是给外部观察者 (人 / 测试 / 监控) 一个明确信号. 关键词:
	// startStallHeartbeat, post-action 卡死兜底观察
	stopStallHeartbeat := r.startStallHeartbeat(task.GetContext(), task)
	defer stopStallHeartbeat()

	if !r.isSimpleQueryWithoutWork() {
		if !utils.IsNil(r.memoryTriage) {
			go func() {
				log.Info("start to handle searching memory for ReActLoop with AI")
				result, err := r.memoryTriage.SearchMemory(task, 5*1024)
				if err != nil {
					log.Warnf("search memory failed: %v", err)
				}
				if task.GetContext().Err() == nil {
					r.PushMemory(result)
				}
			}()
		}
	}

	needSummary := utils.NewBool(false)
LOOP:
	for {
		iterationCount++
		r.currentIterationIndex = iterationCount
		// 主循环每轮推进一次都给 stall heartbeat 打个 tick, 让心跳协程
		// 知道 "我还在动"; 心跳逻辑见 startStallHeartbeat / recordIterationTick.
		// 关键词: 主循环 tick, lastIterationTickAt
		r.recordIterationTick()
		// 迭代上限基于 effectiveIterationCount (有效推进轮数): 有活跃 TODO 但
		// 无 todo_delta 变更的空转轮不计入, 不消耗预算. effectiveIterationCount
		// 在每轮 action 执行后递增, 此处检查时反映的是之前已完成的有效迭代数.
		if r.effectiveIterationCount >= maxIterations {
			// 到达迭代上限: 优先尝试向用户申请临时扩充 (仅在允许交互时).
			// 用户同意 -> 提升 maxIterations 并 continue; 拒绝/不可扩充 -> 软性中断.
			if !r.disableIncreaseIterationCount {
				agreed, extDelta, extErr := r.requestIterationExtension(task, iterationCount, maxIterations)
				if extErr != nil {
					log.Warnf("ReactLoop[%v] request iteration extension error (fallback to soft interrupt): %v", r.loopName, extErr)
				}
				if agreed && extDelta > 0 {
					// 用户同意扩充: 提升上限, 不推进计数, 直接 continue.
					maxIterations += extDelta
					r.UserStatus(
						"我会继续推进，并完成剩余步骤",
						"Continuing with the remaining steps",
						aicommon.WithStatusCode("task.extended"),
					)
					continue
				}
			}
			// 未扩充: 软性中断 (按"自然结束"处理, 非硬错误).
			// applyMaxIterationSoftInterrupt 将活跃 TODO 标记 deferred 并记录快照,
			// finishIterationLoopWithError 跑 onPostIteration hook, 各 loop 的
			// finalize 据此生成总结说明, 框架层按 EmitReActSuccess 上报.
			maxIterErr := utils.Errorf("reached max iterations (%d), stopping %s loop", maxIterations, r.loopName)
			r.applyMaxIterationSoftInterrupt(iterationCount, task, maxIterations)
			r.finishIterationLoopWithError(iterationCount, task, maxIterErr)
			log.Infof("Loop soft-exit on max iterations (%d) for %s loop, waiting for user decision", maxIterations, r.loopName)
			needSummary.SetTo(true)
			break LOOP
		}

		if err := task.GetContext().Err(); err != nil {
			return utils.Errorf("task context done before execute ReActLoop: %v", err)
		}
		r.refreshFastMemoryAsync(task)

		r.UserStatus(
			"正在推进下一步",
			"Moving on to the next step",
			aicommon.WithStatusCode("task.working"),
		)
		var prompt string
		// PE-TASK 缓存优化: 当 task 实现 CacheableUserInputProvider 接口时,
		// 把 PARENT_TASK + CURRENT_TASK + INSTRUCTION 整块当作 frozenUserContext
		// 注入 frozen-block, 让 dynamic 段不再承载 PLAN 阶段的产物。
		// 普通 ReAct loop 的 task 不实现该接口, fallback 走老路径。
		// 关键词: CacheableUserInputProvider, frozenUserContext, PLAN_CONTEXT
		userInputForDynamic := task.GetUserInput()
		var frozenUserContext string
		if provider, ok := task.(aicommon.CacheableUserInputProvider); ok {
			userInputForDynamic, frozenUserContext = provider.GetUserInputSplitForCache()
		}
		// goal-mode finish gate: single application point. Applied here so the
		// operator used to build this iteration's prompt has disallowLoopExit set
		// when the current iteration is below GoalMinIterations, causing finish to
		// be removed from the schema. Idempotent via DisallowNextLoopExit's Once.
		r.ApplyGoalModeGate(operator, iterationCount)
		prompt, finalError = r.generateLoopPrompt(
			nonce,
			userInputForDynamic,
			frozenUserContext,
			nil,
			r.GetCurrentMemoriesContent(),
			operator,
		)
		if finalError != nil {
			r.finishIterationLoopWithError(iterationCount, task, finalError)
			log.Errorf("Failed to generate prompt: %v", finalError)
			needSummary.SetTo(true)
			return finalError
		}
		// Save prompt to file in debug mode
		if r.isDebugModeEnabled() {
			r.savePromptToFile(task, iterationCount, prompt)
		}
		// observation 已通过 emitter.EmitPromptProfile 走 prompt_profile 结构化事件
		// 推送给前端 "上下文成分" 面板, 不再落盘 ASCII 副本以避免污染 task 目录.
		// 关键词: prompt observation 不再落盘, EmitPromptProfile 现役路径

		streamWg := new(sync.WaitGroup)
		/* Generate AI Action */
		loopCalls, stopReason, resultDescriptor, transactionErr := r.callAILoopTransaction(
			streamWg, prompt, nonce, operator,
			r.emitLoopGeneralOutput, r.emitLoopFunctionCallOutput,
		)
		if resultDescriptor != nil {
			log.Debugf("AI loop transaction stop=%s provider_finish=%q", stopReason, resultDescriptor.Snapshot().ProviderFinishReason)
		}
		streamWg.Wait()
		iterationModelThinking := strings.TrimSpace(r.takeModelThinkingForTimeline())
		if transactionErr != nil {
			r.recordModelThinkingTimeline(iterationModelThinking, "", nonce, false)
			r.finishIterationLoopWithError(iterationCount, task, transactionErr)
			log.Errorf("Failed to execute loop: %v", transactionErr)
			needSummary.SetTo(true)
			return transactionErr
		}
		if len(loopCalls) == 0 {
			r.recordModelThinkingTimeline(iterationModelThinking, "", nonce, false)
			finalError = utils.Error("AI loop returned no actions")
			r.finishIterationLoopWithError(iterationCount, task, finalError)
			needSummary.SetTo(true)
			return finalError
		}

		// A native tool-call response needs one assistant plus every matching tool
		// acknowledgement before any action writes to the timeline. Execution below
		// is the same for both response formats.
		var eventSink loopActionEventSink
		if stopReason == LoopStopToolCalls {
			if err := r.appendFunctionCallActionResponse(loopCalls, resultDescriptor); err != nil {
				r.finishIterationLoopWithError(iterationCount, task, err)
				needSummary.SetTo(true)
				return err
			}
			eventSink = r.appendFunctionCallActionEvent
		} else {
			decision := r.Get("last_ai_decision_response")
			if loopCalls[0].Action == nil {
				decision = ""
			}
			r.recordModelThinkingTimeline(iterationModelThinking, decision, nonce, loopCalls[0].Action != nil)
		}

		batch := r.execCalls(loopCalls, iterationCount, task, prompt, done, eventSink)
		operator = batch.operator
		if batch.err != nil || batch.result == loopActionsError {
			finalError = batch.err
			if finalError == nil {
				finalError = utils.Error("action batch failed")
			}
			if !batch.skipErrorFinalization {
				r.finishIterationLoopWithError(iterationCount, task, finalError)
			}
			if !batch.skipErrorSummary {
				needSummary.SetTo(true)
			}
			return finalError
		}
		if batch.result == loopActionsExit || batch.result == loopActionsAsync {
			r.finishIterationLoopWithError(iterationCount, task, nil)
			return nil
		}
		if batch.skipPostIteration {
			continue LOOP
		}
		postOp := r.doneCurrentIteration(iterationCount, task)
		if postOp.ShouldEndIteration() {
			if reason := r.tryFinishSubAgents(); reason != "" {
				operator = newLoopActionHandlerOperator(task)
				operator.Feedback(reason)
				operator.Continue()
				continue LOOP
			}
			log.Infof("Loop ending due to post-iteration operator request: %v", postOp.GetEndReason())
			needSummary.SetTo(true)
			break LOOP
		}
		continue LOOP
	}
	return nil
}

// applyActionExecutionRecord copies the handler's objective execution fact into
// the already-appended history record. Declared ToolNames/ToolCallCount remain
// untouched so diagnostics can still explain what the model attempted.
func (r *ReActLoop) applyActionExecutionRecord(record *ActionRecord, operator *LoopActionHandlerOperator) {
	if r == nil || record == nil || operator == nil {
		return
	}
	executed := operator.GetExecutedToolCallCount()
	if executed <= 0 {
		return
	}
	r.actionHistoryMutex.Lock()
	record.ExecutedToolCallCount += executed
	r.actionHistoryMutex.Unlock()
}

func (r *ReActLoop) doneCurrentIteration(current int, task aicommon.AIStatefulTask) *OnPostIterationOperator {
	operator := newOnPostIterationOperator()
	if r.onPostIteration != nil {
		r.callOnPostIteration(current, task, false, nil, operator)
	}
	return operator
}

func (r *ReActLoop) callOnPostIteration(current int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator) {
	// Phase 1: Run all registered callbacks in order.
	// Callbacks may set flags (e.g. IgnoreError()) and register deferred functions.
	for _, fn := range r.onPostIteration {
		fn(r, current, task, isDone, reason, operator)
	}
	// Phase 2: Run deferred functions after ALL callbacks have completed.
	// This ensures deferred logic can safely check the final operator state
	// (e.g. ShouldIgnoreError()) regardless of callback registration order.
	operator.RunDeferredFuncs()
}

func (r *ReActLoop) finishIterationLoopWithError(current int, task aicommon.AIStatefulTask, err any) *OnPostIterationOperator {
	// Final summaries must see cancelled/unresolved children before task completion.
	r.shutdownSubAgents()
	operator := newOnPostIterationOperator()
	if r.onPostIteration != nil {
		if err != nil {
			r.callOnPostIteration(current, task, true, utils.Errorf("reason: %v", err), operator)
		} else {
			r.callOnPostIteration(current, task, true, nil, operator)
		}
	}
	return operator
}

const (
	// maxIterationInterruptFlagKey 标记本次 loop 因为到达迭代上限被软性中断.
	// finalize / directly-answer hook 用它判断是否要在回答里加"任务中断"框架.
	// 关键词: max iteration 软中断标记
	maxIterationInterruptFlagKey = "__max_iteration_interrupted__"
	// maxIterationInterruptSummaryKey 存储软性中断时"未完成 TODO"的可读快照,
	// 供 finalize hook 在直接回答里明确告诉用户"哪些事情没来得及做".
	// 关键词: max iteration 软中断, 未完成 TODO 交接
	maxIterationInterruptSummaryKey = "__max_iteration_interrupt_summary__"
)

// IsMaxIterationInterrupted 返回本次 loop 是否因为到达迭代上限被软性中断.
func (r *ReActLoop) IsMaxIterationInterrupted() bool {
	if r == nil {
		return false
	}
	b, _ := r.GetVariable(maxIterationInterruptFlagKey).(bool)
	return b
}

// GetMaxIterationInterruptSummary 返回软性中断时记录的"未完成 TODO"可读快照.
// 未发生中断或没有未完成 TODO 时返回空串.
func (r *ReActLoop) GetMaxIterationInterruptSummary() string {
	if r == nil {
		return ""
	}
	return r.Get(maxIterationInterruptSummaryKey)
}

// applyMaxIterationSoftInterrupt 处理到达迭代上限时的"待办回收 + 软性提示":
//  1. 读取当前任务仍开放的 TODO, 记录其可读快照供直接回答引用;
//  2. 复用 ApplyTodoDeltaAndEmit 把这些 TODO 批量关闭为 deferred，并记录 reason
//     (更新 store + 广播 todo_list_update + TODO_DELTA timeline);
//  3. 在 Timeline 追加一条软性中断说明 (非 error 类别), 只报告一次.
//
// 该方法只负责"回收 + 提示", 不生成直接回答 — 直接回答交给 loop 的 finalize
// hook (见各 loop 的 WithOnPostIteraction), 以便复用每个 loop 已有的答复渲染.
//
// 关键词: max iteration 软性中断, 待办 deferred 留痕, 单条软提示, 复用单源 helper
func (r *ReActLoop) applyMaxIterationSoftInterrupt(iterationCount int, task aicommon.AIStatefulTask, _ int) {
	if r == nil || utils.IsNil(task) {
		return
	}

	cfg := r.config
	scope := aicommon.BuildVerificationTodoScope(task)

	var activeItems []aicommon.VerificationTodoItem
	if cfg != nil && !scope.IsZero() {
		activeItems = cfg.ActiveVerificationTodoItemsByScope(scope)
	}

	// 记录未完成 TODO 的可读快照, 供直接回答引用.
	var summaryBuilder strings.Builder
	for _, item := range activeItems {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		summaryBuilder.WriteString("- ")
		summaryBuilder.WriteString(content)
		summaryBuilder.WriteString("\n")
	}
	r.Set(maxIterationInterruptFlagKey, true)
	if summary := strings.TrimSpace(summaryBuilder.String()); summary != "" {
		r.Set(maxIterationInterruptSummaryKey, summary)
	}

	// 把开放 TODO 批量关闭为 deferred: 复用与 todo_delta / 主循环兜底
	// 完全一致的单源 helper, 保证 store 更新 + todo_list_update 广播 + timeline
	// breadcrumb 字节级对齐.
	if cfg != nil && len(activeItems) > 0 {
		reason := "Host execution capacity ended after the recorded attempts; unfinished work is deferred until a later continuation."
		delta := aicommon.BuildDeferredDeltaForOpenTodos(activeItems, reason)
		if delta != nil {
			var timelineHook func(category, line string)
			if invoker := r.GetInvoker(); invoker != nil {
				timelineHook = func(category, line string) {
					invoker.AddToTimeline(category, line)
				}
			}
			aicommon.ApplyTodoDeltaAndEmit(
				cfg,
				r.GetEmitter(),
				task,
				scope,
				iterationCount,
				delta,
				timelineHook,
			)
		}
	}

	// 软性中断的单条 timeline 说明 (非 error 类别, 只报告一次).
	if invoker := r.GetInvoker(); invoker != nil {
		loopName := r.loopName
		if loopName == "" {
			loopName = "general-purpose"
		}
		msg := fmt.Sprintf(
			"[%v] host execution capacity ended and the task was paused (NOT a failure). %d unfinished TODO(s) were preserved as deferred with explicit reasons. A direct answer will summarize what remains; reply \"继续\" to resume, or give a new direction.",
			loopName, len(activeItems),
		)
		invoker.AddToTimeline("execution_paused", msg)
	}
}

func testIsFinished(task aicommon.AIStatefulTask) bool {
	return task.GetStatus() == aicommon.AITaskState_Completed || task.GetStatus() == aicommon.AITaskState_Aborted || task.GetStatus() == aicommon.AITaskState_Skipped
}

// ensureLoopDirectory initializes the loop directory metadata for artifact organization.
// It stores the task directory path and loop name prefix, which are used by
// GetLoopContentDir to construct flat content directories like:
//
//	task_{index}/loop_{name}_action_calls/
//	task_{index}/loop_{name}_prompts/
//	task_{index}/loop_{name}_data/
//
// This avoids the deep nesting of the old structure (task_{index}/loops/{name}/action_calls/).
func (r *ReActLoop) ensureLoopDirectory(task aicommon.AIStatefulTask) string {
	if utils.IsNil(r) || utils.IsNil(task) {
		return ""
	}
	workdir := r.config.GetOrCreateWorkDir()
	if workdir == "" {
		workdir = consts.GetDefaultBaseHomeDir()
	}
	taskIndex := task.GetIndex()
	if taskIndex == "" {
		taskIndex = "0"
	}
	loopName := r.loopName
	if loopName == "" {
		loopName = "unknown_loop"
	}

	taskSemanticId := task.GetSemanticIdentifier()
	taskDir := filepath.Join(workdir, aicommon.BuildTaskDirName(taskIndex, taskSemanticId))
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		log.Errorf("failed to create task directory %s: %v", taskDir, err)
		return ""
	}

	loopPrefix := "loop_" + sanitizeActionFilename(loopName)
	r.Set("task_directory", taskDir)
	r.Set("loop_name_prefix", loopPrefix)

	// For backward compatibility: "loop_directory" now points to the flat data directory
	// for callers that write files directly into the loop directory.
	loopDataDir := filepath.Join(taskDir, loopPrefix+"_data")
	r.Set("loop_directory", loopDataDir)
	return loopDataDir
}

// GetLoopContentDir returns a flat directory for a specific content type within the loop.
// Format: task_{index}/loop_{name}_{contentType}/
// Example: task_1-3/loop_default_action_calls/
//
// The directory is created if it does not exist. This method can be called by any code
// that needs to organize loop-specific artifacts into categorized flat directories.
func (r *ReActLoop) GetLoopContentDir(contentType string) string {
	taskDir := r.Get("task_directory")
	prefix := r.Get("loop_name_prefix")

	if taskDir == "" || prefix == "" {
		// Metadata not initialized yet; try to initialize from current task
		task := r.GetCurrentTask()
		if task == nil {
			return ""
		}
		r.ensureLoopDirectory(task)
		taskDir = r.Get("task_directory")
		prefix = r.Get("loop_name_prefix")
	}

	if taskDir == "" || prefix == "" {
		return ""
	}

	dir := filepath.Join(taskDir, prefix+"_"+contentType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Errorf("failed to create loop content directory %s: %v", dir, err)
		return ""
	}
	return dir
}

func (r *ReActLoop) savePromptToFile(task aicommon.AIStatefulTask, iteration int, prompt string) {
	if utils.IsNil(r) || utils.IsNil(task) {
		return
	}
	emitter := r.GetEmitter()
	if emitter == nil {
		return
	}

	// Use flat loop content directory: task_{index}/loop_{name}_prompts/
	promptDir := r.GetLoopContentDir("prompts")
	if promptDir == "" {
		log.Errorf("failed to get loop content directory for prompts")
		return
	}

	filename := fmt.Sprintf("iteration_%d_prompt_%d.md", iteration, time.Now().Unix())
	filePath := filepath.Join(promptDir, filename)

	var content strings.Builder
	content.WriteString(fmt.Sprintf("# Iteration %d - Generated Prompt\n\n", iteration))
	content.WriteString(fmt.Sprintf("**Loop Name:** %s\n\n", r.loopName))
	content.WriteString(fmt.Sprintf("**Generated at:** %s\n\n", utils.DatetimePretty()))
	content.WriteString("---\n\n")
	content.WriteString(prompt)

	if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
		log.Errorf("failed to save prompt to file: %v", err)
		return
	}
	emitter.EmitPinFilename(filePath)
	log.Infof("saved prompt to file: %s", filePath)
}

func (r *ReActLoop) emitActionExecutionRecord(task aicommon.AIStatefulTask, action *aicommon.Action, iteration int, prompt string, callID ...string) {
	if utils.IsNil(r) || utils.IsNil(task) || utils.IsNil(action) {
		return
	}
	emitter := r.GetEmitter()
	if emitter == nil {
		return
	}

	// Use flat loop content directory: task_{index}/loop_{name}_action_calls/
	actionDir := r.GetLoopContentDir("action_calls")
	if actionDir == "" {
		log.Errorf("failed to get loop content directory for action_calls")
		return
	}

	actionName := action.Name()
	if actionName == "" {
		actionName = action.ActionType()
	}
	filename := fmt.Sprintf("%d_%s.md", iteration, sanitizeActionFilename(actionName))
	if len(callID) > 0 && callID[0] != "" {
		filename = fmt.Sprintf("%d_%s_%s.md", iteration, sanitizeActionFilename(actionName), sanitizeActionFilename(callID[0]))
	}
	filePath := filepath.Join(actionDir, filename)

	content := r.buildActionExecutionMarkdown(actionName, action.GetParams(), action.GetString("human_readable_thought"), prompt, r.isDebugModeEnabled())
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		log.Errorf("failed to save action execution record to file: %v", err)
		return
	}
	emitter.EmitPinFilename(filePath)
	log.Infof("saved action execution record to file: %s", filePath)
}

func (r *ReActLoop) buildActionExecutionMarkdown(actionName string, params map[string]any, thought string, prompt string, includePrompt bool) string {
	var content strings.Builder
	content.WriteString("# Action Call Record\n\n")
	content.WriteString("## Action\n\n")
	content.WriteString("- Name: " + actionName + "\n")
	content.WriteString("- Human Readable Thought: " + thought + "\n\n")
	content.WriteString("## Params\n\n")
	content.WriteString("```json\n")
	content.WriteString(string(utils.Jsonify(params)))
	content.WriteString("\n```\n\n")
	if includePrompt {
		content.WriteString("## Prompt\n\n")
		content.WriteString("```\n")
		content.WriteString(prompt)
		content.WriteString("\n```\n")
	}
	return content.String()
}

func (r *ReActLoop) isDebugModeEnabled() bool {
	// Check debug_mode variable first
	value := r.GetVariable("debug_mode")
	if value != nil {
		if enabled, ok := value.(bool); ok && enabled {
			return true
		}
		if strings.EqualFold(utils.InterfaceToString(value), "true") {
			return true
		}
	}
	return false
}

// extractToolNameFromAction 按优先级从 action 参数里抽取工具名，供执行历史使用。
// 字段优先级:
//  1. directly_call_tool_name 顶层
//  2. next_action.directly_call_tool_name (legacy 兼容)
//  3. tool_require_payload (require_tool 路径)
//  4. tool_name / tool 通用兜底
//
// 全部命中为空返回空串, 表示该 action 不是 tool 调用类。
//
// 关键词: extractToolNameFromAction, tool_name 抽取优先级
func extractToolNameFromAction(action *aicommon.Action) string {
	names := extractToolNamesFromAction(action)
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

// extractToolNamesFromAction 按模型声明顺序提取单次或批量工具名。批量字段只从
// canonical action object 解码，避免 ActionMaker 的扁平兼容字段把数组 item 串线。
func extractToolNamesFromAction(action *aicommon.Action) []string {
	if utils.IsNil(action) {
		return nil
	}
	params := action.GetParams()
	roots := []map[string]any{params}
	if nextAction := params.GetObject("next_action"); len(nextAction) > 0 {
		roots = append(roots, nextAction)
	}
	for _, root := range roots {
		for _, field := range []string{"directly_call_tool_calls", "tool_require_calls"} {
			raw, ok := root[field]
			if !ok || raw == nil {
				continue
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var items []struct {
				ToolName string `json:"tool_name"`
			}
			if err := json.Unmarshal(encoded, &items); err != nil {
				continue
			}
			names := make([]string, 0, len(items))
			for _, item := range items {
				if name := strings.TrimSpace(item.ToolName); name != "" {
					names = append(names, name)
				}
			}
			if len(names) > 0 {
				return names
			}
		}
	}
	if name := strings.TrimSpace(action.GetString("directly_call_tool_name")); name != "" {
		return []string{name}
	}
	nextAction := action.GetInvokeParams("next_action")
	if nextAction != nil {
		if name := strings.TrimSpace(nextAction.GetString("directly_call_tool_name")); name != "" {
			return []string{name}
		}
		if name := strings.TrimSpace(nextAction.GetString("tool_require_payload")); name != "" {
			return []string{name}
		}
	}
	if name := strings.TrimSpace(action.GetString("tool_require_payload")); name != "" {
		return []string{name}
	}
	if name := strings.TrimSpace(action.GetString("tool_name")); name != "" {
		return []string{name}
	}
	if name := strings.TrimSpace(action.GetString("tool")); name != "" {
		return []string{name}
	}
	return nil
}

func cloneActionParams(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		cloned := make(map[string]any, len(params))
		for key, value := range params {
			cloned[key] = value
		}
		return cloned
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		cloned = make(map[string]any, len(params))
		for key, value := range params {
			cloned[key] = value
		}
	}
	return cloned
}

func sanitizeActionFilename(name string) string {
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result += string(r)
		} else {
			result += "_"
		}
	}
	if result == "" {
		return "unknown"
	}
	return result
}
