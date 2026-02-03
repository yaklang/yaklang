package aicommon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (a *ToolCaller) intervalReviewContext(
	ctx context.Context, reviewCancel func(),
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	onAICanceled func(any),
) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("interval review context panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if utils.IsNil(a.intervalReviewHandler) {
		return
	}

	reviewDuration := a.GetIntervalReviewDuration()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(reviewDuration)
			select {
			case <-ctx.Done():
				return
			default:
				shouldContinue, err := a.intervalReviewHandler(ctx, tool, params, stdoutSnapshot, stderrSnapshot)
				if err != nil {
					log.Errorf("interval review handler failed: %v", err)
					continue
				}
				if !shouldContinue {
					reviewCancel()
					if !utils.IsNil(onAICanceled) {
						onAICanceled(fmt.Sprintf("interval review handler failed: %v", err))
					}
					return
				}
			}
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
	a.intervalReviewContext(ctx, reviewCancel, tool, params, stdoutSnapshot, stderrSnapshot, onAICanceled)
}

func (a *ToolCaller) invoke(
	tool *aitool.Tool,
	params aitool.InvokeParams,
	userCancel func(reason any),
	reportError func(err any),
	stdoutWriter, stderrWriter io.Writer,
	stdoutSnapshotBuffer, stderrSnapshotBuffer *bytes.Buffer,
) (*aitool.ToolResult, error) {
	c := a.config
	e := a.emitter

	seq := c.AcquireId()
	db := c.GetDB()
	if db != nil {
		if ret, ok := yakit.GetToolCallCheckpoint(db, c.GetRuntimeId(), seq); ok {
			if ret.Finished {
				return aiddb.AiCheckPointGetToolResult(ret), nil
			}
		}
	}
	toolCheckpoint := c.CreateToolCallCheckpoint(seq)
	err := c.SubmitCheckpointRequest(toolCheckpoint, map[string]any{
		"tool_name": tool.Name,
		"param":     params,
	})
	if err != nil {
		return nil, err
	}

	epm := c.GetEndpointManager()
	ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_CALL_WATCHER)
	e.EmitToolCallWatcher(a.callToolId, ep.GetId(), tool, params)

	// Use task context if available (for proper cancellation), otherwise fall back to config context
	var baseCtx context.Context
	if a.task != nil {
		if statefulTask, ok := a.task.(AIStatefulTask); ok {
			baseCtx = statefulTask.GetContext()
		} else {
			baseCtx = c.GetContext()
		}
	} else {
		baseCtx = c.GetContext()
	}

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	newToolCallRes := func() *aitool.ToolResult {
		return &aitool.ToolResult{
			Param:       params,
			Name:        tool.Name,
			Description: tool.Description,
			ToolCallID:  a.callToolId,
		}
	}

	toolCallSuccess := func(result *aitool.ToolExecutionResult) (*aitool.ToolResult, error) {
		res := newToolCallRes()
		res.Success = true
		res.Data = result
		err = c.SubmitCheckpointResponse(toolCheckpoint, res)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	toolCallErr := func(err error) (*aitool.ToolResult, error) {
		reportError(err)
		res := newToolCallRes()
		res.Error = fmt.Sprintf("tool execution failed: %v", err)
		return res, err
	}

	toolCallCancel := func(result *aitool.ToolExecutionResult, err error) (*aitool.ToolExecutionResult, error) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, nil
		}
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

	stdoutWriter.Write([]byte(fmt.Sprintf("invoking tool[%v] ...\n", tool.Name))) // 确保触发执行卡片，优化体验

	log.Infof("start to invoke tool[%s] with params: %v", tool.Name, params)

	if !utils.IsNil(a.intervalReviewHandler) {
		intervalStart := make(chan struct{})
		go func() {
			close(intervalStart)
			a.intervalReviewContext(
				ctx, cancel,
				tool, params,
				stdoutSnapshotBuffer.Bytes(),
				stderrSnapshotBuffer.Bytes(),
				userCancel,
			)
		}()
		<-intervalStart
	}

	execResult, execErr := tool.InvokeWithParams(
		params,
		aitool.WithStdout(stdoutWriter),
		aitool.WithStderr(stderrWriter),
		aitool.WithContext(ctx),
		aitool.WithErrorCallback(toolCallErr),
		aitool.WithResultCallback(toolCallSuccess),
		aitool.WithCancelCallback(toolCallCancel),
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
			RuntimeID: a.callToolId,
			FeedBacker: func(result *ypb.ExecResult) error {
				// 处理 risk 消息
				risk, _ := handleRiskMessage(result)
				if risk != nil {
					e.EmitYakitRisk(risk.ID, risk.Title)
				}
				// 过滤文件 Stat/Read 等高频消息，避免对前端造成压力
				if shouldIgnoreExecResultForEmit(result) {
					return nil
				}
				e.EmitYakitExecResult(result)
				return nil
			},
		}),
	)
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

// shouldIgnoreExecResultForEmit checks if the ExecResult should be ignored
// to reduce gRPC pressure on the frontend.
// It filters out high-frequency file STATUS (Stat) messages.
func shouldIgnoreExecResultForEmit(result *ypb.ExecResult) bool {
	if result == nil || !result.IsMessage || len(result.Message) == 0 {
		return false
	}

	var yakitMsg yaklib.YakitMessage
	if err := json.Unmarshal(result.Message, &yakitMsg); err != nil {
		return false
	}

	if yakitMsg.Type != "log" {
		return false
	}

	if len(yakitMsg.Content) == 0 {
		return false
	}

	var logInfo yaklib.YakitLog
	if err := json.Unmarshal(yakitMsg.Content, &logInfo); err != nil {
		return false
	}

	// filter out file level logs with STATUS action (yakit.fileStatusAction)
	// STATUS is called for every file during traversal in find_file.yak, grep.yak etc.
	// This causes massive gRPC messages when scanning large directories
	//
	// Message structure (from YakitClient.YakitDraw -> YakitFile):
	// YakitMessage{Type: "log", Content: YakitLog{Level: "file", Data: `{"action":"STATUS",...}`}}
	if logInfo.Level == "file" && logInfo.Data != "" {
		var fileData = make(map[string]any)
		if err := json.Unmarshal([]byte(logInfo.Data), &fileData); err != nil {
			// cannot parse Data as JSON, don't filter (safe default)
			return false
		}

		action := utils.InterfaceToString(fileData["action"])
		if action == "STATUS" {
			return true
		}
	}

	return false
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
				Url             string `json:"Url"`
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
