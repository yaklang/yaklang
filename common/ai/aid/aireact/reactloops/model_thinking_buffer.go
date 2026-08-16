package reactloops

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const (
	timelineModelThinkingReplayTagName = "TIMELINE_MODEL_THINKING_V1"
	timelineModelThinkingReplayVersion = 1
)

type modelThinkingReplayRecord struct {
	Version          int    `json:"v"`
	ReasoningContent string `json:"reasoning_content"`
	Content          string `json:"content"`
}

// promptProjectedTimelineRuntime is deliberately optional. Production ReAct
// implements it, while alternate runtimes and existing test doubles continue
// to receive the display-only model_thinking entry through AddToTimeline.
type promptProjectedTimelineRuntime interface {
	AddToTimelineWithPromptProjection(entry, displayContent, promptContent string)
}

// bindDecisionResponseCapture scopes reasoning and action capture to one
// concrete AIResponse. CallAITransaction invokes the post handler again for a
// retry, so resetting here guarantees that only the final accepted attempt is
// eligible for replay.
func (r *ReActLoop) bindDecisionResponseCapture(resp *aicommon.AIResponse) {
	if r == nil || resp == nil {
		return
	}
	r.resetModelThinkingBuffer()
	resp.SetOnReasonChunk(func(chunk []byte) {
		r.appendModelThinkingChunk(chunk)
	})
	resp.SetOnOutputFinished(func(output string) {
		r.Set("last_ai_decision_response", output)
		if r.config != nil {
			r.config.CallAIResponseOutputFinishedCallback(output)
		}
	})
}

func (r *ReActLoop) resetModelThinkingBuffer() {
	if r == nil {
		return
	}
	r.modelThinkingMutex.Lock()
	defer r.modelThinkingMutex.Unlock()
	r.modelThinkingBuf.Reset()
}

func (r *ReActLoop) appendModelThinkingChunk(chunk []byte) {
	if r == nil || len(chunk) == 0 {
		return
	}
	r.modelThinkingMutex.Lock()
	defer r.modelThinkingMutex.Unlock()
	r.modelThinkingBuf.Write(chunk)
}

// takeModelThinkingForTimeline returns accumulated model reasoning for the
// current AI transaction and clears the buffer.
func (r *ReActLoop) takeModelThinkingForTimeline() string {
	if r == nil {
		return ""
	}
	r.modelThinkingMutex.Lock()
	defer r.modelThinkingMutex.Unlock()
	s := strings.TrimSpace(r.modelThinkingBuf.String())
	r.modelThinkingBuf.Reset()
	return s
}

func buildModelThinkingReplayProjection(nonce, reasoning, content string) (string, error) {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" || strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("reasoning replay requires non-empty reasoning and content")
	}
	nonce = sanitizeModelThinkingReplayNonce(nonce)
	record, err := json.Marshal(modelThinkingReplayRecord{
		Version:          timelineModelThinkingReplayVersion,
		ReasoningContent: reasoning,
		Content:          content,
	})
	if err != nil {
		return "", fmt.Errorf("marshal model thinking replay: %w", err)
	}
	return fmt.Sprintf(
		"<|%s_%s|>\n%s\n<|%s_END_%s|>",
		timelineModelThinkingReplayTagName,
		nonce,
		record,
		timelineModelThinkingReplayTagName,
		nonce,
	), nil
}

func sanitizeModelThinkingReplayNonce(nonce string) string {
	var result strings.Builder
	for _, char := range nonce {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			result.WriteRune(char)
		}
	}
	if result.Len() == 0 {
		return "replay"
	}
	return result.String()
}

func (r *ReActLoop) recordModelThinkingTimeline(reasoning, actionContent, nonce string, replay bool) {
	if r == nil {
		return
	}
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return
	}
	invoker := r.GetInvoker()
	if invoker == nil {
		return
	}
	if replay {
		if projection, err := buildModelThinkingReplayProjection(nonce, reasoning, actionContent); err == nil {
			if projected, ok := invoker.(promptProjectedTimelineRuntime); ok {
				projected.AddToTimelineWithPromptProjection(TimelineEntryModelThinking, reasoning, projection)
				return
			}
		}
	}
	invoker.AddToTimeline(TimelineEntryModelThinking, reasoning)
}
