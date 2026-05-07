package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// ModelThinkingPromptMaxRunes is how much merged thinking text is injected
	// into the next prompt (tail slice).
	ModelThinkingPromptMaxRunes = 5000
)

func (r *ReActLoop) appendModelThinkingChunk(chunk []byte) {
	if r == nil || len(chunk) == 0 {
		return
	}
	task := r.GetCurrentTask()
	if task == nil {
		return
	}
	cfg := r.config
	if cfg == nil {
		return
	}
	db := cfg.GetDB()
	if db == nil {
		return
	}

	row := &schema.AIReActThinkingChunk{
		TaskId:     task.GetId(),
		RuntimeId:  cfg.GetRuntimeId(),
		LoopName:   r.loopName,
		ByteLen:    len(chunk),
		Content:    string(chunk),
	}
	if p, ok := cfg.(interface{ GetPersistentSessionID() string }); ok {
		row.PersistentSessionId = strings.TrimSpace(p.GetPersistentSessionID())
	}
	_ = yakit.SaveAIReActThinkingChunk(db, row)
}

// PriorModelThinkingForPrompt returns the tail of merged thinking text for prompt injection (from DB).
func (r *ReActLoop) PriorModelThinkingForPrompt() string {
	if r == nil {
		return ""
	}
	cfg := r.config
	if cfg == nil {
		return ""
	}
	db := cfg.GetDB()
	if db == nil {
		return ""
	}
	var sid, rt string
	if p, ok := cfg.(interface{ GetPersistentSessionID() string }); ok {
		sid = strings.TrimSpace(p.GetPersistentSessionID())
	}
	rt = strings.TrimSpace(cfg.GetRuntimeId())
	merged, err := yakit.LoadAIReActThinkingAggregated(db, r.loopName, sid, rt)
	if err != nil {
		log.Warnf("LoadAIReActThinkingAggregated failed (loop=%s): %v", r.loopName, err)
		return ""
	}
	return tailStringByRunes(strings.TrimSpace(merged), ModelThinkingPromptMaxRunes)
}

func tailStringByRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[len(runes)-maxRunes:])
}
