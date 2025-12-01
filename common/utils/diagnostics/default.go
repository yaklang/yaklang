package diagnostics

import "sync"

var defaultRecorder struct {
	mu       sync.RWMutex
	recorder *Recorder
}

func init() {
	defaultRecorder.recorder = NewRecorder()
}

func DefaultRecorder() *Recorder {
	defaultRecorder.mu.RLock()
	defer defaultRecorder.mu.RUnlock()
	return defaultRecorder.recorder
}

func ReplaceDefault(rec *Recorder) *Recorder {
	if rec == nil {
		rec = NewRecorder()
	}
	defaultRecorder.mu.Lock()
	old := defaultRecorder.recorder
	defaultRecorder.recorder = rec
	defaultRecorder.mu.Unlock()
	return old
}

func ResetDefaultRecorder() *Recorder {
	rec := NewRecorder()
	ReplaceDefault(rec)
	return rec
}
