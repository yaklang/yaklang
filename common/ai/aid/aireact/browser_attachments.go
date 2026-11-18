package aireact

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type browserAttachmentState struct {
	mu       sync.RWMutex
	bindings []aicommon.AttachedBrowserBinding
}

// Bindings are immutable after parsing. Attachments add to the session state;
// removal is handled by an explicit session operation, never by a new resource.
func (s *browserAttachmentState) add(bindings []aicommon.AttachedBrowserBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{}, len(s.bindings)+len(bindings))
	references := make(map[string]string, len(s.bindings)+len(bindings))
	for _, binding := range s.bindings {
		seen[binding.DeviceID] = struct{}{}
		if reference := strings.ToLower(strings.TrimSpace(binding.Reference)); reference != "" {
			references[reference] = binding.DeviceID
		}
	}
	pending := make([]aicommon.AttachedBrowserBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.DeviceID = strings.TrimSpace(binding.DeviceID)
		binding.Reference = strings.TrimSpace(binding.Reference)
		if binding.DeviceID == "" {
			return fmt.Errorf("attached browser has no device ID")
		}
		if binding.Reference == "" {
			return fmt.Errorf("attached browser device %q has no runtime reference", binding.DeviceID)
		}
		if _, exists := seen[binding.DeviceID]; exists {
			continue
		}
		referenceKey := strings.ToLower(binding.Reference)
		if existingDeviceID, exists := references[referenceKey]; exists && existingDeviceID != binding.DeviceID {
			return fmt.Errorf(
				"browser reference %q is ambiguous between attached devices %q and %q",
				binding.Reference,
				existingDeviceID,
				binding.DeviceID,
			)
		}
		seen[binding.DeviceID] = struct{}{}
		references[referenceKey] = binding.DeviceID
		pending = append(pending, binding)
	}
	s.bindings = append(s.bindings, pending...)
	return nil
}

func (s *browserAttachmentState) get() []aicommon.AttachedBrowserBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bindings
}

// AddBrowserAttachments implements aicommon.BrowserAttachmentSession.
func (r *ReAct) AddBrowserAttachments(bindings []aicommon.AttachedBrowserBinding) error {
	if r == nil {
		return nil
	}
	return r.browserAttachments.add(bindings)
}

// GetBrowserAttachments implements aicommon.BrowserAttachmentProvider.
func (r *ReAct) GetBrowserAttachments() []aicommon.AttachedBrowserBinding {
	if r == nil {
		return nil
	}
	return r.browserAttachments.get()
}
