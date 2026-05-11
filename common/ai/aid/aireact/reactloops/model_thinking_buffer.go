package reactloops

import (
	"strings"
)

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
