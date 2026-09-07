package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	ToolCallAction_Enough_Cancel = "enough-cancel"
	ToolCallAction_Finish        = "finish"
)

type toolOutputBuffer struct {
	mu      sync.Mutex
	sampler *boundedHeadTailBuffer
}

func (b *toolOutputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	if b.sampler == nil {
		b.sampler = newBoundedHeadTailBuffer(toolOutputSnapshotMaxBytes)
	}
	sampler := b.sampler
	b.mu.Unlock()
	return sampler.Write(p)
}

func (b *toolOutputBuffer) Snapshot() []byte {
	b.mu.Lock()
	sampler := b.sampler
	b.mu.Unlock()
	if sampler == nil {
		return nil
	}
	return sampler.Snapshot()
}

func (b *toolOutputBuffer) Bytes() []byte {
	return b.Snapshot()
}

func (b *toolOutputBuffer) Len() int {
	b.mu.Lock()
	sampler := b.sampler
	b.mu.Unlock()
	if sampler == nil {
		return 0
	}
	return sampler.Len()
}

func staticSnapshot(snapshot []byte) func() []byte {
	return func() []byte {
		return snapshot
	}
}

func (a *ToolCaller) intervalReviewContext(
	ctx context.Context, reviewCancel func(),
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot func() []byte,
	onAICanceled func(any),
) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("interval review context panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if stdoutSnapshot == nil {
		stdoutSnapshot = func() []byte { return nil }
	}
	if stderrSnapshot == nil {
		stderrSnapshot = func() []byte { return nil }
	}

	if utils.IsNil(a.intervalReviewHandler) {
		return
	}

	reviewDuration := a.GetIntervalReviewDuration()
	startedAt := time.Now()
	reviewCount := 0
	toolName := ""
	if tool != nil && tool.Tool != nil {
		toolName = tool.Name
	}
	emitProgressReview := func(payload ToolCallProgressReviewPayload) {
		if a.emitter == nil {
			return
		}
		payload.Tool = toolName
		payload.IntervalSeconds = reviewDuration.Seconds()
		payload.ElapsedSeconds = time.Since(startedAt).Seconds()
		if _, err := a.emitter.EmitToolCallProgressReview(a.callToolId, payload); err != nil {
			log.Warnf("emit tool progress review event failed: %v", err)
		}
	}
	emitProgressReview(ToolCallProgressReviewPayload{
		Phase:          schema.TOOL_CALL_PROGRESS_REVIEW_PHASE_SCHEDULED,
		NextReviewAtMS: startedAt.Add(reviewDuration).UnixMilli(),
	})

	timer := time.NewTimer(reviewDuration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			reviewCount++
			stdout := stdoutSnapshot()
			stderr := stderrSnapshot()
			reviewStartedAt := time.Now()
			emitProgressReview(ToolCallProgressReviewPayload{
				Phase:              schema.TOOL_CALL_PROGRESS_REVIEW_PHASE_STARTED,
				ReviewCount:        reviewCount,
				StdoutSnapshotSize: len(stdout),
				StderrSnapshotSize: len(stderr),
			})

			reviewCtx := withToolCallIntervalReviewMetadata(ctx, ToolCallIntervalReviewMetadata{
				ToolExecutionStartedAt: startedAt,
				ReviewCount:            reviewCount,
			})
			shouldContinue, err := a.intervalReviewHandler(reviewCtx, tool, params, stdout, stderr, a.callExpectations)
			reviewDurationMS := time.Since(reviewStartedAt).Milliseconds()
			if err != nil {
				log.Errorf("interval review handler failed: %v", err)
				emitProgressReview(ToolCallProgressReviewPayload{
					Phase:            schema.TOOL_CALL_PROGRESS_REVIEW_PHASE_FAILED,
					ReviewCount:      reviewCount,
					ReviewDurationMS: reviewDurationMS,
					Decision:         "continue",
					Error:            utils.ShrinkString(err.Error(), 480),
					NextReviewAtMS:   time.Now().Add(reviewDuration).UnixMilli(),
				})
				if ctx.Err() != nil {
					return
				}
				timer.Reset(reviewDuration)
				continue
			}

			decision := "continue"
			if !shouldContinue {
				decision = "cancel"
			}
			nextReviewAtMS := int64(0)
			if shouldContinue {
				nextReviewAtMS = time.Now().Add(reviewDuration).UnixMilli()
			}
			emitProgressReview(ToolCallProgressReviewPayload{
				Phase:            schema.TOOL_CALL_PROGRESS_REVIEW_PHASE_COMPLETED,
				ReviewCount:      reviewCount,
				ReviewDurationMS: reviewDurationMS,
				Decision:         decision,
				NextReviewAtMS:   nextReviewAtMS,
			})
			if !shouldContinue {
				reviewCancel()
				if !utils.IsNil(onAICanceled) {
					onAICanceled("tool execution cancelled by interval progress review")
				}
				return
			}
			timer.Reset(reviewDuration)
		}
	}
}

// IntervalReviewContext is the public wrapper for intervalReviewContext.
// This allows external packages (like tests) to call the interval review logic directly.
func (a *ToolCaller) IntervalReviewContext(
	ctx context.Context, reviewCancel func(),
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	onAICanceled func(any),
) {
	a.intervalReviewContext(
		ctx, reviewCancel,
		tool, params,
		staticSnapshot(stdoutSnapshot),
		staticSnapshot(stderrSnapshot),
		onAICanceled,
	)
}

func (a *ToolCaller) GetCallExpectations() string {
	return a.callExpectations
}

func (a *ToolCaller) invoke(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	userCancel func(reason any),
	reportError func(err any),
	stdoutWriter, stderrWriter io.Writer,
	stdoutSnapshotBuffer, stderrSnapshotBuffer *toolOutputBuffer,
	finalizeResults ...func(*aitool.ToolResult) error,
) (*aitool.ToolResult, error) {
	// A ToolCaller can recursively enter invoke after a review override. Track
	// replay for the current invocation only; the outer pipeline uses this bit to
	// suppress session/stat notifications when no plugin callback ran.
	a.checkpointReplayed = false
	c := a.config
	e := a.emitter
	if err := toolCallerContextErr(a.ctx); err != nil {
		return nil, err
	}
	var finalizeResult func(*aitool.ToolResult) error
	if len(finalizeResults) > 0 {
		finalizeResult = finalizeResults[0]
	}

	seq := a.checkpointSeq
	if seq <= 0 {
		seq = c.AcquireId()
	}
	if ret, ok := yakit.GetToolCallCheckpoint(c.GetDB(), c.GetRuntimeId(), seq); ok {
		if a.batchID != "" {
			stored := aiddb.AiCheckPointGetRequestParams(ret)
			storedParam := stored.GetObject("param")
			storedIndex, hasStoredIndex := stored["call_index"]
			if stored.GetString("batch_id") != a.batchID ||
				!hasStoredIndex ||
				utils.InterfaceToInt(storedIndex) != a.batchIndex ||
				stored.GetString("call_tool_id") != a.callToolId ||
				stored.GetString("tool_name") != tool.Name ||
				string(utils.Jsonify(storedParam)) != string(utils.Jsonify(params)) {
				return nil, utils.Errorf(
					"tool batch checkpoint identity mismatch: expected batch=%s index=%d call=%s tool=%s params=%s; stored batch=%s index=%d call=%s tool=%s params=%s",
					a.batchID,
					a.batchIndex,
					a.callToolId,
					tool.Name,
					utils.Jsonify(params),
					stored.GetString("batch_id"),
					utils.InterfaceToInt(storedIndex),
					stored.GetString("call_tool_id"),
					stored.GetString("tool_name"),
					utils.Jsonify(storedParam),
				)
			}
		}
		if ret.Finished {
			a.checkpointReplayed = true
			res := aiddb.AiCheckPointGetToolResult(ret)
			if finalizeResult != nil && res != nil {
				if err := finalizeResult(res); err != nil {
					return res, err
				}
				if err := c.SubmitCheckpointResponse(ret, res); err != nil {
					return res, err
				}
			}
			return res, nil
		}
	}
	toolCheckpoint := c.CreateToolCallCheckpoint(seq)
	checkpointRequest := map[string]any{
		"tool_name": tool.Name,
		"param":     params,
	}
	if a.batchID != "" {
		checkpointRequest["batch_id"] = a.batchID
		checkpointRequest["call_index"] = a.batchIndex
		checkpointRequest["call_tool_id"] = a.callToolId
	}
	err := c.SubmitCheckpointRequest(toolCheckpoint, checkpointRequest)
	if err != nil {
		return nil, err
	}

	epm := c.GetEndpointManager()
	watcherCheckpointSeq := a.watcherCheckpointSeq
	a.watcherCheckpointSeq = 0
	ep := epm.CreateEndpointWithEventTypeAndSeq(schema.EVENT_TYPE_TOOL_CALL_WATCHER, watcherCheckpointSeq)
	e.EmitToolCallWatcher(a.callToolId, ep.GetId(), tool, params)

	// Prefer the caller-scoped context. Batch children derive it from both the
	// action context and task context, so cancelling either one interrupts the
	// plugin. Scalar callers also populate a.ctx and keep their old semantics.
	var baseCtx = a.ctx
	if baseCtx == nil && a.task != nil {
		if statefulTask, ok := a.task.(AIStatefulTask); ok {
			baseCtx = statefulTask.GetContext()
		} else {
			baseCtx = c.GetContext()
		}
	} else if baseCtx == nil {
		baseCtx = c.GetContext()
	}

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	newToolCallRes := func() *aitool.ToolResult {
		return &aitool.ToolResult{
			Param:            params,
			Name:             tool.Name,
			Description:      tool.Description,
			ToolCallID:       a.callToolId,
			CallExpectations: a.callExpectations,
		}
	}

	toolCallSuccess := func(result *aitool.ToolExecutionResult) (*aitool.ToolResult, error) {
		res := newToolCallRes()
		res.Success = true
		res.Data = result
		return res, nil
	}

	toolCallErr := func(err error) (*aitool.ToolResult, error) {
		reportError(err)
		res := newToolCallRes()
		res.Error = fmt.Sprintf("tool invocation protocol failed: %v", err)
		return res, err
	}

	toolCallCancel := func(result *aitool.ToolExecutionResult, err error) (*aitool.ToolExecutionResult, error) {
		return result, err
	}

	go func() {
		ep.WaitContext(ctx)
		userSuggestion := ep.GetParams()
		switch userSuggestion.GetString("suggestion") {
		case string(ToolCallAction_Enough_Cancel):
			cancel()
			userCancel("user cancelled the tool call, continuing with the next task")
		case ToolCallAction_Finish:
		default:
			reportError(fmt.Sprintf("user did not select a valid action, cannot continue tool call: %v", userSuggestion))
		}
	}()

	noRuntimeId := !params.Has("runtime_id")
	if noRuntimeId {
		params.Set("runtime_id", a.runtimeId)
	}

	log.Infof("start to invoke tool[%s] with params: %v", tool.Name, params)

	if !utils.IsNil(a.intervalReviewHandler) {
		// InvokeWithParams performs small in-place normalizations while validating
		// (notably temporarily removing runtime_id). Interval review runs in a
		// parallel goroutine and renders params into its prompt, so it must observe
		// an immutable deep snapshot instead of racing those tool-owned mutations.
		intervalReviewParams := cloneEndpointParams(params)
		intervalStart := make(chan struct{})
		go func() {
			close(intervalStart)
			a.intervalReviewContext(
				ctx, cancel,
				tool, intervalReviewParams,
				stdoutSnapshotBuffer.Snapshot,
				stderrSnapshotBuffer.Snapshot,
				userCancel,
			)
		}()
		<-intervalStart
	}

	refreshHTTPFlowCount := func() {
		count := yakit.CountHTTPFlowByRuntimeID(consts.GetGormProjectDatabase(), a.callToolId)
		if count > 0 {
			e.EmitYakitHTTPFlowCount(a.callToolId, count)
		}
	}

	refreshRiskCount := func() {
		count, _ := yakit.CountRiskByRuntimeId(consts.GetGormProjectDatabase(), a.callToolId)
		if count > 0 {
			e.EmitYakitRiskCount(a.callToolId, count)
		}
	}

	unsubscribe := schema.SubscribeRuntimeScopedBroadcast(a.callToolId, func(event *schema.RuntimeScopedBroadcastEvent) {
		if event.Type == schema.RuntimeScopedBroadcastTypeHTTPFlow {
			refreshHTTPFlowCount()
		}

		if event.Type == schema.RuntimeScopedBroadcastTypeRisk {
			refreshRiskCount()
		}
	})
	defer func() {
		unsubscribe()
		refreshHTTPFlowCount()
		refreshRiskCount()
		NotifySessionSnapshotRuntimeRefresh(a.config, a.callToolId)
	}()

	var browserTracker interface {
		TrackBrowserSession(id string)
		UntrackBrowserSession(id string)
	}
	if c != nil {
		browserTracker = c.GetBrowserSessionTracker()
	}
	// boundRisks 收集本次工具调用实际 emit / 绑定到 runtime 的 risk (漏洞).
	// 漏洞主要由 cybersecurity-risk 等 yak 插件工具产生, 工具执行结束后据此异步
	// 提交 risk_feedback 价值反馈 (交小模型判定是否误报). FeedBacker 可能被工具执行
	// 协程并发回调, 故用锁保护.
	var (
		boundRisksMu sync.Mutex
		boundRisks   []*schema.Risk
	)
	runtimeCfg := &aitool.ToolRuntimeConfig{
		RuntimeID:             a.callToolId,
		ProjectDatabase:       a.config.GetDB(),
		BrowserSessionTracker: browserTracker,
		FeedBacker: func(result *ypb.ExecResult) error {
			// 处理 risk 消息
			risk, _ := handleRiskMessage(result)
			if risk != nil {
				e.EmitYakitRisk(risk.ID, risk.Title, risk.RuntimeId)
				boundRisksMu.Lock()
				boundRisks = append(boundRisks, risk)
				boundRisksMu.Unlock()
				// Append a compact summary to the session-level reported-risks
				// store so the model sees a "已报告漏洞清单" in every subsequent
				// prompt and avoids duplicate cybersecurity-risk calls.
				a.config.AppendReportedRisk(risk)
			}
			httpFlow, _ := handleHTTPFlowMessage(result)
			if httpFlow != nil {
				e.EmitYakitHTTPFlow(httpFlow.RuntimeId, httpFlow.HiddenIndex)
				return nil
			}
			if path, ok := handleFileWriteMessage(result); ok {
				NotifySessionSnapshotFileWrite(a.config, path)
			}
			// 过滤文件 Stat/Read 等高频消息，避免对前端造成压力
			if shouldIgnoreExecResultForEmit(result) {
				return nil
			}
			e.EmitYakitExecResult(result)
			return nil
		},
	}
	if sessionProvider, ok := a.config.(interface{ GetPersistentSessionID() string }); ok {
		runtimeCfg.PersistentSessionID = sessionProvider.GetPersistentSessionID()
	}
	if statefulTask, ok := a.task.(AIStatefulTask); ok && statefulTask != nil {
		runtimeCfg.CurrentTaskUserInput = statefulTask.GetOriginUserInput()
	} else if a.invokeRuntime != nil {
		if currentTask := a.invokeRuntime.GetCurrentTask(); currentTask != nil {
			runtimeCfg.CurrentTaskUserInput = currentTask.GetOriginUserInput()
		}
	}
	// When the runtime is bound to a server-authorized target (Focus release or
	// conversation-audit session), route tool-emitted risks to the platform
	// result sink instead of process-local SQLite.
	if resultRuntime := LegionResultRuntimeFromConfig(a.config); resultRuntime != nil {
		runtimeCfg.RiskSaveHandler = func(_ context.Context, risk *schema.Risk) error {
			return submitToolRiskToPlatformSink(resultRuntime, risk)
		}
	}
	// Everything above prepares observers and runtime metadata. Cancellation can
	// race any of those steps, so make the final callback boundary explicit: a
	// cancelled child must never enter plugin code after being admitted earlier.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	execResult, execErr := tool.InvokeWithParams(
		params,
		aitool.WithStdout(stdoutWriter),
		aitool.WithStderr(stderrWriter),
		aitool.WithContext(ctx),
		aitool.WithErrorCallback(toolCallErr),
		aitool.WithResultCallback(toolCallSuccess),
		aitool.WithCancelCallback(toolCallCancel),
		aitool.WithRuntimeConfig(runtimeCfg),
		aitool.WithOutputCapture(false),
	)
	invokeCancelled := errors.Is(execErr, context.Canceled) ||
		errors.Is(execErr, context.DeadlineExceeded) ||
		ctx.Err() != nil
	if invokeCancelled {
		if execErr == nil {
			execErr = ctx.Err()
			if execErr == nil {
				execErr = context.Canceled
			}
		}
		if execResult != nil {
			execResult.Success = false
			if execResult.Error == "" {
				execResult.Error = fmt.Sprintf("tool invocation cancelled: %v", execErr)
			}
		}
	}
	if execErr != nil {
		// Preflight failures have no script callback, so without this line their
		// artifact would misleadingly say COMBINED OUTPUT: (empty). Mirror the final
		// error into the regular stderr stream; this keeps live logs and saved
		// artifacts useful even when the structured error event is unavailable.
		_, _ = fmt.Fprintf(stderrWriter, "[error] %v\n", execErr)
	}
	if execResult != nil && finalizeResult != nil && !invokeCancelled {
		if finalizeErr := finalizeResult(execResult); finalizeErr != nil {
			if execErr == nil {
				execErr = finalizeErr
			} else {
				execErr = errors.Join(execErr, finalizeErr)
			}
		}
	}
	if execResult != nil && !invokeCancelled {
		if checkpointErr := c.SubmitCheckpointResponse(toolCheckpoint, execResult); checkpointErr != nil {
			// The callback has already produced a result envelope. Checkpoint
			// persistence is an infrastructure observation and must not rewrite
			// protocol completion into a tool failure.
			log.Warnf("failed to persist completed tool result checkpoint: %v", checkpointErr)
		}
	}

	// Some failures happen outside the tool callback (for example JSON Schema
	// validation or cancellation). Those paths can
	// return a ToolResult and an error without passing through WithErrorCallback.
	// Always report the final aggregated error here so the UI receives tool_call_error
	// instead of a silent done card. The caller's handler is guarded by sync.Once, so
	// callback-time errors remain single events even when they are reported here too.
	if execErr != nil {
		reportError(execErr)
	}

	// 工具调用结束、runtime 已绑定漏洞之后, 异步提交 risk_feedback 价值反馈 (AI 自判).
	// 这是对 cybersecurity-risk 等 "报漏洞" 工具插件的通用埋点, 与 loop 内 generate_risk
	// 动作路径互补. 非阻塞投递 + 内部 recover, 绝不影响工具调用结果与主流程.
	boundRisksMu.Lock()
	risksSnapshot := boundRisks
	boundRisksMu.Unlock()
	if len(risksSnapshot) > 0 {
		if cfg, ok := c.(*Config); ok {
			cfg.SubmitToolRiskFeedback(tool.Name, risksSnapshot)
		}
	}

	ep.ActiveWithParams(ctx, map[string]any{"suggestion": "finish"})
	reqs := map[string]any{"suggestion": "finish"}
	e.EmitInteractiveRelease(ep.GetId(), reqs)
	c.CallAfterInteractiveEventReleased(ep.GetId(), reqs)

	if execResult != nil && noRuntimeId {
		if r, ok := execResult.Param.(aitool.InvokeParams); ok {
			if r.Has("runtime_id") {
				delete(r, "runtime_id")
			}
		}
	}

	return execResult, execErr
}

// submitToolRiskToPlatformSink routes a tool-emitted risk into the
// server-authorized result sink (Legion JobRisk) via the bound LegionResultRuntime.
// When no runtime is bound (e.g. desktop client without a server context),
// RiskSaveHandler stays nil and the caller falls back to local SQLite.
func submitToolRiskToPlatformSink(runtime LegionResultRuntime, risk *schema.Risk) error {
	if runtime == nil {
		return errors.New("AI result runtime is unavailable")
	}
	if risk == nil {
		return errors.New("risk is required")
	}
	target := strings.TrimSpace(risk.Url)
	if target == "" {
		target = strings.TrimSpace(runtime.AuthorizedTarget())
	}
	_, err := runtime.Execute("result.risk", map[string]any{
		"verified":          true,
		"target":            target,
		"title":             risk.Title,
		"risk_type":         risk.RiskType,
		"severity":          risk.Severity,
		"parameter":         risk.Parameter,
		"payload":           risk.Payload,
		"description":       risk.Description,
		"solution":          risk.Solution,
		"details":           risk.Details,
		"request_evidence":  risk.QuotedRequest,
		"response_evidence": risk.QuotedResponse,
	})
	if err != nil {
		return fmt.Errorf("submit risk to platform result sink: %w", err)
	}
	return nil
}

// shouldIgnoreExecResultForEmit checks if the ExecResult should be ignored
// to reduce gRPC pressure on the frontend.
// It filters out high-frequency file STATUS (Stat) messages.
func shouldIgnoreExecResultForEmit(result *ypb.ExecResult) bool {
	fileData, ok := parseYakitFileExecResult(result)
	if !ok {
		return false
	}
	action := utils.InterfaceToString(fileData["action"])
	return action == yaklib.Status_Action || action == "STATUS"
}

func parseYakitFileExecResult(result *ypb.ExecResult) (map[string]any, bool) {
	if result == nil || !result.IsMessage || len(result.Message) == 0 {
		return nil, false
	}

	var yakitMsg yaklib.YakitMessage
	if err := json.Unmarshal(result.Message, &yakitMsg); err != nil {
		return nil, false
	}
	if yakitMsg.Type != "log" || len(yakitMsg.Content) == 0 {
		return nil, false
	}

	var logInfo yaklib.YakitLog
	if err := json.Unmarshal(yakitMsg.Content, &logInfo); err != nil {
		return nil, false
	}
	if logInfo.Level != "file" || strings.TrimSpace(logInfo.Data) == "" {
		return nil, false
	}

	var fileData map[string]any
	if err := json.Unmarshal([]byte(logInfo.Data), &fileData); err != nil {
		return nil, false
	}
	return fileData, true
}

// handleFileWriteMessage detects yakit.File fileWriteAction telemetry (action=WRITE).
func handleFileWriteMessage(result *ypb.ExecResult) (path string, ok bool) {
	fileData, parsed := parseYakitFileExecResult(result)
	if !parsed {
		return "", false
	}
	action := strings.ToUpper(strings.TrimSpace(utils.InterfaceToString(fileData["action"])))
	if action != yaklib.Write_Action {
		return "", false
	}
	path = strings.TrimSpace(utils.InterfaceToString(fileData["path"]))
	if path == "" {
		return "", false
	}
	return path, true
}

func handleRiskMessage(result *ypb.ExecResult) (*schema.Risk, error) {
	// 解析消息
	msg := &yaklib.YakitMessage{}
	err := json.Unmarshal(result.Message, msg)
	if err != nil {
		return nil, err
	}

	// 解析yakit日志
	var logInfo *yaklib.YakitLog
	if msg.Type == "log" {
		logInfoIns := &yaklib.YakitLog{}
		err := json.Unmarshal(msg.Content, logInfoIns)
		if err != nil {
			return nil, utils.Errorf("unmarshal log info failed: %v", err)
		}
		logInfo = logInfoIns
	}

	// 解析 risk 信息
	if logInfo != nil {
		if logInfo.Level == "json-risk" {
			// 使用中间结构体处理时间戳
			type riskJSON struct {
				CreatedAt int64  `json:"CreatedAt"`
				UpdatedAt int64  `json:"UpdatedAt"`
				DeletedAt *int64 `json:"DeletedAt,omitempty"`

				Description     string `json:"Description"`
				Hash            string `json:"Hash"`
				Host            string `json:"Host"`
				IP              string `json:"IP"`
				Id              uint   `json:"Id"`
				Port            int    `json:"Port"`
				Request         []byte `json:"Request"`
				Response        []byte `json:"Response"`
				RiskType        string `json:"RiskType"`
				RiskTypeVerbose string `json:"RiskTypeVerbose"`
				RuntimeId       string `json:"RuntimeId"`
				Severity        string `json:"Severity"`
				Title           string `json:"Title"`
				TitleVerbose    string `json:"TitleVerbose"`
				Url             string `json:"Url"`
				Parameter       string `json:"Parameter"`
				Payload         string `json:"Payload"`
			}

			var riskData riskJSON
			err := json.Unmarshal([]byte(logInfo.Data), &riskData)
			if err != nil {
				return nil, utils.Errorf("unmarshal risk info failed: %v", err)
			}

			// 转换为 schema.Risk
			risk := &schema.Risk{
				Hash:            riskData.Hash,
				IP:              riskData.IP,
				Url:             riskData.Url,
				Port:            riskData.Port,
				Host:            riskData.Host,
				Title:           riskData.Title,
				Description:     riskData.Description,
				RiskType:        riskData.RiskType,
				RiskTypeVerbose: riskData.RiskTypeVerbose,
				RuntimeId:       riskData.RuntimeId,
				Severity:        riskData.Severity,
				Parameter:       riskData.Parameter,
				Payload:         riskData.Payload,
			}
			risk.ID = riskData.Id
			risk.CreatedAt = time.Unix(riskData.CreatedAt, 0)
			risk.UpdatedAt = time.Unix(riskData.UpdatedAt, 0)

			// 处理 Request 和 Response（如果有的话）
			if len(riskData.Request) > 0 {
				risk.QuotedRequest = string(riskData.Request)
			}
			if len(riskData.Response) > 0 {
				risk.QuotedResponse = string(riskData.Response)
			}

			return risk, nil
		}
		return nil, utils.Errorf("unknown log level: %s", logInfo.Level)
	}
	return nil, utils.Errorf("unknown message type: %s", msg.Type)
}

func handleHTTPFlowMessage(result *ypb.ExecResult) (*schema.HTTPFlow, error) {
	if result == nil || !result.IsMessage || len(result.Message) == 0 {
		return nil, utils.Errorf("result is not a yakit log message")
	}

	msg := &yaklib.YakitMessage{}
	if err := json.Unmarshal(result.Message, msg); err != nil {
		return nil, err
	}
	if msg.Type != "log" {
		return nil, utils.Errorf("unknown message type: %s", msg.Type)
	}

	logInfo := &yaklib.YakitLog{}
	if err := json.Unmarshal(msg.Content, logInfo); err != nil {
		return nil, utils.Errorf("unmarshal log info failed: %v", err)
	}
	if logInfo.Level != "json-httpflow" {
		return nil, utils.Errorf("unknown log level: %s", logInfo.Level)
	}

	var flow struct {
		RuntimeId   string `json:"runtime_id"`
		HiddenIndex string `json:"hidden_index"`
	}
	if err := json.Unmarshal([]byte(logInfo.Data), &flow); err != nil {
		return nil, utils.Errorf("unmarshal httpflow info failed: %v", err)
	}
	if flow.HiddenIndex == "" {
		return nil, utils.Errorf("httpflow hidden_index is empty")
	}

	return &schema.HTTPFlow{
		RuntimeId:   flow.RuntimeId,
		HiddenIndex: flow.HiddenIndex,
	}, nil
}
