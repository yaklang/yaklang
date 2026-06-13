package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

type Endpoint struct {
	id              string
	sig             *EndpointSignal
	reviewType      schema.EventType
	activeParams    aitool.InvokeParams
	reviewMaterials aitool.InvokeParams

	// createdAtMs 是端点创建 (发起审批) 的毫秒时间戳, 用于计算审批时延.
	createdAtMs int64
	// approvalMeta 记录本次审批"运行时真相": 谁做的决定 (source)、是否真的需要
	// 人工/模型介入 (required)、原因 (reason). 由 DoWaitAgreeWithPolicy 在各释放
	// 分支就地写入, 供价值评估区分 policy 与 source (例如 YOLO 下仍人工审批).
	approvalMeta *ApprovalDecisionMeta

	// seq and checkpoint for recovering
	seq        int64
	checkpoint *schema.AiCheckpoint
}

// ApprovalDecisionMeta 描述一次审批决定的运行时来源信息.
type ApprovalDecisionMeta struct {
	// Source 取 human / policy / model_judge / rule / timeout_fallback.
	Source string
	// Required 表示这次是否真的需要人工或模型判定 (manual / ai 高风险=true;
	// yolo / auto / ai 低中风险 自动放行=false).
	Required bool
	// Reason 是机器可读的决定原因 (如 auto_approve_by_yolo_policy).
	Reason string
	// DecidedAtMs 是做出决定的毫秒时间戳.
	DecidedAtMs int64
}

func (e *Endpoint) GetCreatedAtMs() int64 {
	return e.createdAtMs
}

// SetApprovalMeta 由审批链路在确定决定来源时调用 (就地记录运行时真相).
func (e *Endpoint) SetApprovalMeta(source string, required bool, reason string) {
	e.approvalMeta = &ApprovalDecisionMeta{
		Source:      source,
		Required:    required,
		Reason:      reason,
		DecidedAtMs: time.Now().UnixMilli(),
	}
}

func (e *Endpoint) GetApprovalMeta() *ApprovalDecisionMeta {
	return e.approvalMeta
}

func (e *Endpoint) GetSeq() int64 {
	return e.seq
}

func (e *Endpoint) GetCheckpoint() *schema.AiCheckpoint {
	if e.checkpoint == nil {
		e.checkpoint = &schema.AiCheckpoint{
			Seq: e.seq,
		}
	}
	return e.checkpoint
}

func (e *Endpoint) SetReviewMaterials(
	params aitool.InvokeParams) {
	if !utils.IsNil(params) {
		e.reviewMaterials = params
	}
}

func (e *Endpoint) GetReviewMaterials() aitool.InvokeParams {
	params := make(aitool.InvokeParams)
	for k, v := range e.reviewMaterials {
		params[k] = v
	}
	return params
}

func (e *Endpoint) WaitContext(ctx context.Context) {
	err := e.sig.WaitContext(ctx)
	if err != nil {
		log.Errorf("Failed to wait for endpoint %s: %v", e.id, err)
		return
	}
}

func (e Endpoint) ReleaseContext(ctx context.Context) {
	e.sig.ActiveContext(ctx)
}

func (e *Endpoint) WaitTimeoutSeconds(i float64) bool {
	return e.WaitTimeout(time.Duration(i * float64(time.Second)))
}

// 新增的 WaitTimeout 方法
func (e *Endpoint) WaitTimeout(timeout time.Duration) bool {
	return e.sig.WaitTimeout(timeout) == nil
}

func (e *Endpoint) Wait() {
	e.sig.Wait()
}

// 修改后的 GetParams 方法，添加锁保护
func (e *Endpoint) GetParams() aitool.InvokeParams {
	params := make(aitool.InvokeParams)
	for k, v := range e.activeParams {
		params[k] = v
	}
	return params
}

func (e *Endpoint) SetParams(params aitool.InvokeParams) {
	if !utils.IsNil(params) {
		e.activeParams = params
	}
}

func (e *Endpoint) SetDefaultSuggestion(suggestion string) {
	e.activeParams["suggestion"] = suggestion
}

func (e *Endpoint) SetDefaultSuggestionContinue() {
	e.SetDefaultSuggestion("continue")
}

func (e *Endpoint) SetDefaultSuggestionEnd() {
	e.SetDefaultSuggestion("end")
}

func (e *Endpoint) SetDefaultSuggestionYes() {
	e.SetDefaultSuggestion("yes")
}

func (e *Endpoint) SetDefaultSuggestionNo() {
	e.SetDefaultSuggestion("no")
}

func (e *Endpoint) ActiveWithParams(ctx context.Context, params aitool.InvokeParams) {
	if !utils.IsNil(params) {
		e.activeParams = params
	}
	e.sig.ActiveAsyncContext(ctx)
}

func (e *Endpoint) Release() {
	e.sig.ActiveAsyncContext(context.Background())
}

func (e *Endpoint) GetId() string {
	return e.id
}

func (e *Endpoint) GetReviewType() schema.EventType {
	return e.reviewType
}
