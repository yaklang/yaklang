package core

import (
	"sync"
	"sync/atomic"
)

// Opcode parse-coverage instrumentation.
//
// This is a test-support hook, not part of decompilation logic: when recording is enabled, every
// opcode that reaches calcOpcodeStackInfo (the single per-instruction chokepoint of the stack
// simulator) is counted. A coverage test can then assert that the supported test corpus exercises
// every JVM opcode the decompiler is expected to parse.
//
// The disabled path is lock-free: a single relaxed atomic load per instruction. The mutex and map
// are only touched while recording is on (driven by a dedicated, serial coverage test), so the
// concurrent m2 scan harness pays nothing in normal runs.
var (
	opcodeRecordingOn atomic.Bool
	opcodeHitMu       sync.Mutex
	opcodeHitCounts   map[int]int
)

// EnableOpcodeHitRecording starts (or restarts) opcode hit recording, clearing any prior counts.
func EnableOpcodeHitRecording() {
	opcodeHitMu.Lock()
	opcodeHitCounts = map[int]int{}
	opcodeHitMu.Unlock()
	opcodeRecordingOn.Store(true)
}

// DisableOpcodeHitRecording stops recording. Collected counts remain readable until the next Enable.
func DisableOpcodeHitRecording() {
	opcodeRecordingOn.Store(false)
}

// RecordedOpcodeHits returns a copy of the opcode -> hit-count map collected since the last Enable.
func RecordedOpcodeHits() map[int]int {
	opcodeHitMu.Lock()
	defer opcodeHitMu.Unlock()
	out := make(map[int]int, len(opcodeHitCounts))
	for k, v := range opcodeHitCounts {
		out[k] = v
	}
	return out
}

// recordOpcodeHit counts one processed opcode. Inlined cost when disabled is one atomic load.
func recordOpcodeHit(op int) {
	if !opcodeRecordingOn.Load() {
		return
	}
	opcodeHitMu.Lock()
	if opcodeHitCounts != nil {
		opcodeHitCounts[op]++
	}
	opcodeHitMu.Unlock()
}
