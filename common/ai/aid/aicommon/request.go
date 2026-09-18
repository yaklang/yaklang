package aicommon

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

type ImageData struct {
	Data     []byte
	IsBase64 bool
}

type AIRequest struct {
	taskIndex              string
	detachCheckpoint       bool
	prompt                 string
	startTime              time.Time
	seqId                  int64
	saveCheckpointCallback func(CheckpointCommitHandler)
	onAcquireSeq           func(int64)
	imageDataList          []*ImageData
	modelTier              string
	callerLabel            string
	ctx                    context.Context

	// extraSpecOpts carries additional aispec.AIConfigOption values that
	// AIChatToAICallbackType will append to its option list when calling the
	// underlying AI service. This is used by functioncall mode to inject
	// WithTools / WithToolChoice / WithToolCallCallback.
	extraSpecOpts []aispec.AIConfigOption

	// enableToolCallArgumentsStream, when true, tells aicaller.go to wire
	// up ToolCallArgumentsStreamHandler → resp.EmitOutputStream so that
	// tool_call arguments flow through the same output channel as regular
	// content. Used by functioncall mode to unify the action parsing pipeline.
	enableToolCallArgumentsStream bool
}

func (a *AIRequest) GetStartTime() time.Time {
	if a == nil {
		return time.Time{}
	}
	return a.startTime
}

func (a *AIRequest) SetStartTime(t time.Time) {
	if a == nil {
		return
	}
	a.startTime = t
}

func (a *AIRequest) GetSeqId() int64 {
	if a == nil {
		return 0
	}
	return a.seqId
}

func NewAIRequest(prompt string, opt ...AIRequestOption) *AIRequest {
	req := &AIRequest{
		prompt:        prompt,
		startTime:     time.Now(),
		imageDataList: make([]*ImageData, 0),
	}
	for _, i := range opt {
		i(req)
	}
	return req
}

type AIRequestOption func(req *AIRequest)

// WithAIRequest_DetachCheckpoint marks an auxiliary AI request as independent
// from the coordinator's replay sequence. Progress/interval reviews use this so
// a timing-dependent number of reviews cannot shift later deterministic
// checkpoints on recovery.
func WithAIRequest_DetachCheckpoint() AIRequestOption {
	return func(req *AIRequest) {
		req.SetDetachCheckpoint(true)
	}
}

func (a *AIRequest) HaveSaveCheckpointCallback() bool {
	if a == nil {
		return false
	}
	return a.saveCheckpointCallback != nil
}

func (a *AIRequest) CallSaveCheckpointCallback(handler CheckpointCommitHandler) {
	if a == nil || a.saveCheckpointCallback == nil {
		return
	}
	a.saveCheckpointCallback(handler)
}

func (a *AIRequest) GetImageList() []*ImageData {
	if a == nil {
		return nil
	}
	return a.imageDataList
}

func (a *AIRequest) GetTaskIndex() string {
	return a.taskIndex
}

func (a *AIRequest) SetTaskIndex(taskIndex string) {
	a.taskIndex = taskIndex
}

func (ai *AIRequest) SetDetachCheckpoint(b bool) {
	ai.detachCheckpoint = b
}

func (ai *AIRequest) IsDetachedCheckpoint() bool {
	return ai.detachCheckpoint
}

type CheckpointCommitHandler func() (*schema.AiCheckpoint, error)

func (r *AIRequest) GetPrompt() string {
	return r.prompt
}

func (r *AIRequest) SetPrompt(prompt string) {
	r.prompt = prompt
}

func (r *AIRequest) CallOnAcquireSeq(seq int64) {
	if r == nil || r.onAcquireSeq == nil {
		return
	}
	if r.onAcquireSeq != nil {
		r.onAcquireSeq(seq)
	}
}

func WithAIRequest_SaveCheckpointCallback(callback func(CheckpointCommitHandler)) AIRequestOption {
	return func(req *AIRequest) {
		req.saveCheckpointCallback = callback
	}
}

func WithAIRequest_OnAcquireSeq(callback func(int64)) AIRequestOption {
	return func(req *AIRequest) {
		req.onAcquireSeq = callback
	}
}

func WithAIRequest_SeqId(i int64) AIRequestOption {
	return func(req *AIRequest) {
		req.seqId = i
	}
}

func WithAIRequest_ImageData(data *ImageData) AIRequestOption {
	return func(req *AIRequest) {
		if req.imageDataList == nil {
			req.imageDataList = make([]*ImageData, 0, 1)
		}
		req.imageDataList = append(req.imageDataList, data)
	}
}

func (a *AIRequest) GetModelTier() string {
	if a == nil {
		return ""
	}
	return a.modelTier
}

func (a *AIRequest) SetModelTier(tier string) {
	if a == nil {
		return
	}
	a.modelTier = tier
}

func (a *AIRequest) GetCallerLabel() string {
	if a == nil {
		return ""
	}
	return a.callerLabel
}

func (a *AIRequest) SetCallerLabel(label string) {
	if a == nil {
		return
	}
	a.callerLabel = label
}

// GetContext returns the request-scoped context when the caller supplied one.
// It lets concurrent child transactions be cancelled independently instead of
// inheriting only the much longer-lived session config context.
func (a *AIRequest) GetContext() context.Context {
	if a == nil {
		return nil
	}
	return a.ctx
}

func WithAIRequest_Context(ctx context.Context) AIRequestOption {
	return func(req *AIRequest) {
		req.ctx = ctx
	}
}

func WithAIRequest_CallerLabel(label string) AIRequestOption {
	return func(req *AIRequest) {
		req.callerLabel = label
	}
}

// WithAIRequest_ExtraSpecOpts sets additional aispec.AIConfigOption values
// that AIChatToAICallbackType will append to its option list when calling the
// underlying AI service. Used by functioncall mode to inject WithTools /
// WithToolChoice / WithToolCallCallback.
func WithAIRequest_ExtraSpecOpts(opts ...aispec.AIConfigOption) AIRequestOption {
	return func(req *AIRequest) {
		req.extraSpecOpts = append(req.extraSpecOpts, opts...)
	}
}

// GetExtraSpecOpts returns the extra aispec options carried by this request.
func (a *AIRequest) GetExtraSpecOpts() []aispec.AIConfigOption {
	if a == nil {
		return nil
	}
	return a.extraSpecOpts
}

// WithAIRequest_EnableToolCallArgumentsStream tells aicaller.go to stream
// tool_call arguments through resp.EmitOutputStream (in addition to the
// ToolCallCallback). Used by functioncall mode so the postHandler can parse
// tool_call arguments via the same ExtractActionFromStream path as text mode.
func WithAIRequest_EnableToolCallArgumentsStream() AIRequestOption {
	return func(req *AIRequest) {
		req.enableToolCallArgumentsStream = true
	}
}

// IsToolCallArgumentsStreamEnabled returns whether the request asked
// aicaller.go to stream tool_call arguments into the output channel.
func (a *AIRequest) IsToolCallArgumentsStreamEnabled() bool {
	if a == nil {
		return false
	}
	return a.enableToolCallArgumentsStream
}
