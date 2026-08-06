package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// midtermMemoryProvider is implemented by the invoker (typically *aireact.ReAct)
// to consume pending perception-snapshot-based midterm recall queries and
// return the rendered content. This interface bridges the reactloops → aireact
// boundary without a circular import.
type midtermMemoryProvider interface {
	ConsumeAndSearchMidtermMemory() string
}

// refreshMidtermMemoryAsync loads midterm archive content asynchronously,
// mirroring how regular memory is loaded via a goroutine. It consumes the
// pending perception snapshot (summary + topics + keywords) to build search
// queries — exactly the same basis that regular memory uses for its search.
//
// It skips launching a new search if one is already in flight, avoiding
// concurrent duplicate searches from stacking up.
func (r *ReActLoop) refreshMidtermMemoryAsync() {
	if r == nil {
		return
	}
	r.midtermMemoryMu.Lock()
	if r.midtermMemorySearchInFlight {
		r.midtermMemoryMu.Unlock()
		return
	}
	r.midtermMemorySearchInFlight = true
	r.midtermMemoryMu.Unlock()

	go func() {
		defer func() {
			r.midtermMemoryMu.Lock()
			r.midtermMemorySearchInFlight = false
			r.midtermMemoryMu.Unlock()
		}()
		r.refreshMidtermMemory()
	}()
}

// refreshMidtermMemory consumes the pending perception snapshot from the
// invoker and stores the rendered midterm archive content on the loop, so it
// can be appended to InjectedMemory at prompt-build time — exactly like
// regular memory.
func (r *ReActLoop) refreshMidtermMemory() {
	if r == nil {
		return
	}
	invoker := r.GetInvoker()
	if utils.IsNil(invoker) {
		return
	}
	provider, ok := invoker.(midtermMemoryProvider)
	if !ok {
		return
	}

	content := provider.ConsumeAndSearchMidtermMemory()
	r.midtermMemoryMu.Lock()
	r.currentMidtermMemory = content
	r.midtermMemoryMu.Unlock()

	if strings.TrimSpace(content) != "" {
		log.Debugf("midterm memory refreshed for loop: %d bytes", len(content))
	}
}

// GetCurrentMidtermMemory returns the cached midterm archive content.
func (r *ReActLoop) GetCurrentMidtermMemory() string {
	if r == nil {
		return ""
	}
	r.midtermMemoryMu.Lock()
	defer r.midtermMemoryMu.Unlock()
	return r.currentMidtermMemory
}

// appendMidtermToMemory appends midterm archive content to the regular memory
// string so that both are rendered in the dynamic section's InjectedMemory block,
// at the bottom of the prompt — exactly like regular memory.
func appendMidtermToMemory(memory, midterm string) string {
	memory = strings.TrimSpace(memory)
	midterm = strings.TrimSpace(midterm)
	if midterm == "" {
		return memory
	}
	if memory == "" {
		return midterm
	}
	return memory + "\n" + midterm
}
