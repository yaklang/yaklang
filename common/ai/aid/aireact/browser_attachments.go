package aireact

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type browserAttachmentState struct {
	mu       sync.RWMutex
	bindings []aicommon.AttachedBrowserBinding
}

// Bindings are immutable after parsing. Attachments add to the session state;
// removal is handled by an explicit session operation, never by a new resource.
func (s *browserAttachmentState) add(bindings []aicommon.AttachedBrowserBinding) {
	s.mu.Lock()
	seen := make(map[string]struct{}, len(s.bindings)+len(bindings))
	for _, binding := range s.bindings {
		seen[binding.DeviceID] = struct{}{}
	}
	for _, binding := range bindings {
		if _, exists := seen[binding.DeviceID]; exists {
			continue
		}
		seen[binding.DeviceID] = struct{}{}
		s.bindings = append(s.bindings, binding)
	}
	s.mu.Unlock()
}

func (s *browserAttachmentState) get() []aicommon.AttachedBrowserBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bindings
}

// AddBrowserAttachments implements aicommon.BrowserAttachmentSession.
func (r *ReAct) AddBrowserAttachments(bindings []aicommon.AttachedBrowserBinding) {
	if r == nil {
		return
	}
	r.browserAttachments.add(bindings)
}

// GetBrowserAttachments implements aicommon.BrowserAttachmentProvider.
func (r *ReAct) GetBrowserAttachments() []aicommon.AttachedBrowserBinding {
	if r == nil {
		return nil
	}
	return r.browserAttachments.get()
}
