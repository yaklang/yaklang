package yakvm

import (
	"sync"
	"sync/atomic"
)

type mapMemberCallKey struct {
	caller string
	callee string
}

type mapMemberCallHandlers map[mapMemberCallKey]func(interface{}) interface{}

// Registration is rare; root frames and their synchronous/asynchronous children
// retain an immutable snapshot. Updating a VM must not change an existing frame's
// handlers. Build the snapshot lazily so loading a plugin can register all of its
// handlers without copying the table after each registration.
type mapMemberCallHandlerRegistry struct {
	mu       sync.Mutex
	handlers mapMemberCallHandlers
	snapshot atomic.Pointer[mapMemberCallHandlers]
}

func (r *mapMemberCallHandlerRegistry) register(key mapMemberCallKey, h func(interface{}) interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = make(mapMemberCallHandlers)
	}
	r.handlers[key] = h
	r.snapshot.Store(nil)
}

func (r *mapMemberCallHandlerRegistry) loadSnapshot() mapMemberCallHandlers {
	if snapshot := r.snapshot.Load(); snapshot != nil {
		return *snapshot
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if snapshot := r.snapshot.Load(); snapshot != nil {
		return *snapshot
	}
	handlers := make(mapMemberCallHandlers, len(r.handlers))
	for key, handler := range r.handlers {
		handlers[key] = handler
	}
	r.snapshot.Store(&handlers)
	return handlers
}

func (v *Frame) execHijackMapMemberCallHandler(caller, callee string, origin interface{}) interface{} {
	handler, ok := v.hijackMapMemberCallHandlers[mapMemberCallKey{caller, callee}]
	if !ok {
		return origin
	}
	return handler(origin)
}
