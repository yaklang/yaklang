package aicommon

import (
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

type ImageData struct {
	Data     []byte
	IsBase64 bool
}

type PromptFallback func(expectedContextSize int, currentContextSize int, compressionLevel int) (string, error)

type AIRequest struct {
	taskIndex              string
	detachCheckpoint       bool
	prompt                 string
	source                 string
	promptFallback         PromptFallback
	startTime              time.Time
	seqId                  int64
	saveCheckpointCallback func(CheckpointCommitHandler)
	onAcquireSeq           func(int64)
	imageDataList          []*ImageData
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

func (a *AIRequest) HaveSaveCheckpointCallback() bool {
	if a == nil {
		return false
	}
	return a.saveCheckpointCallback != nil
}

func (a *AIRequest) GetPromptFallback() PromptFallback {
	if a == nil {
		return nil
	}
	return a.promptFallback
}

func (a *AIRequest) SetPromptFallback(fallback PromptFallback) {
	if a == nil {
		return
	}
	a.promptFallback = fallback
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

func (r *AIRequest) GetSource() string {
	if r == nil {
		return ""
	}
	return r.source
}

func (r *AIRequest) SetSource(source string) {
	if r == nil {
		return
	}
	r.source = source
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

func WithAIRequest_PromptFallback(fallback PromptFallback) AIRequestOption {
	return func(req *AIRequest) {
		req.SetPromptFallback(fallback)
	}
}

func WithAIRequest_Source(source string) AIRequestOption {
	return func(req *AIRequest) {
		req.SetSource(source)
	}
}
