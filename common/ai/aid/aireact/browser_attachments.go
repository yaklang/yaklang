package aireact

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type browserAttachmentState struct {
	mu       sync.RWMutex
	bindings []aicommon.AttachedBrowserBinding
}

// Bindings are immutable after parsing. Updates replace the entire selection;
// readers never modify the returned slice or its elements.
func (s *browserAttachmentState) set(bindings []aicommon.AttachedBrowserBinding) {
	s.mu.Lock()
	s.bindings = bindings
	s.mu.Unlock()
}

func (s *browserAttachmentState) get() []aicommon.AttachedBrowserBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bindings
}

// SetBrowserAttachments implements aicommon.BrowserAttachmentSession.
func (r *ReAct) SetBrowserAttachments(bindings []aicommon.AttachedBrowserBinding) {
	if r == nil {
		return
	}
	r.browserAttachments.set(bindings)
}

// GetBrowserAttachments implements aicommon.BrowserAttachmentProvider.
func (r *ReAct) GetBrowserAttachments() []aicommon.AttachedBrowserBinding {
	if r == nil {
		return nil
	}
	return r.browserAttachments.get()
}
