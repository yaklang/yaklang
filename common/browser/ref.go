package browser

import (
	"fmt"
	"strings"
	"sync"
)

type RefEntry struct {
	BackendNodeID int
	Role          string
	Name          string
	Nth           int
	Selector      string
	FrameID       string
}

type RefMap struct {
	mu      sync.RWMutex
	refs    map[string]*RefEntry
	counter int
}

func NewRefMap() *RefMap {
	return &RefMap{
		refs: make(map[string]*RefEntry),
	}
}

func (rm *RefMap) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.refs = make(map[string]*RefEntry)
	rm.counter = 0
}

func (rm *RefMap) Assign(entry *RefEntry) string {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.counter++
	ref := fmt.Sprintf("e%d", rm.counter)
	rm.refs[ref] = entry
	return ref
}

func (rm *RefMap) Get(ref string) (*RefEntry, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	entry, ok := rm.refs[ref]
	return entry, ok
}

func (rm *RefMap) Count() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.refs)
}

func ParseRef(selectorOrRef string) (string, bool) {
	s := strings.TrimSpace(selectorOrRef)
	if strings.HasPrefix(s, "@") {
		return s[1:], true
	}
	if strings.HasPrefix(s, "ref=") {
		return s[4:], true
	}
	return "", false
}
