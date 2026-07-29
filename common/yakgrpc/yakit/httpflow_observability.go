package yakit

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

const (
	// HTTPFlowTimingRegistrySize bounds process memory even when a producer runs
	// indefinitely. IDs naturally overwrite older slots.
	HTTPFlowTimingRegistrySize = 8192
	// HTTPFlowTimingQuerySampleLimit bounds diagnostic response amplification.
	HTTPFlowTimingQuerySampleLimit = 64
	// Keep independent high-water marks for a small bounded set of recently
	// active project databases. This prevents a late async write from an old
	// project from clearing the current project's observation state.
	httpFlowHighWaterSlotCount = 16
)

// HTTPFlowPersistTiming is a copy-safe view of one recently persisted flow.
// Unix milliseconds are intentional: JavaScript can represent them exactly,
// unlike Unix nanoseconds.
type HTTPFlowPersistTiming struct {
	ID                             uint64
	DatabaseIdentity               string
	ProjectGeneration              uint64
	RequestHijackAtUnixMs          int64
	ResponseMirrorAtUnixMs         int64
	FlowBuiltAtUnixMs              int64
	PersistEnqueuedAtUnixMs        int64
	PersistStartedAtUnixMs         int64
	PersistedAtUnixMs              int64
	DatabaseChangeDetectedAtUnixMs int64
}

type httpFlowTimingSlot struct {
	mu    sync.RWMutex
	value HTTPFlowPersistTiming
	valid bool
}

type HTTPFlowPipelineHighWater struct {
	DatabaseIdentity        string
	ProjectGeneration       uint64
	LatestPersistedID       uint64
	LatestPersistedAtUnixMs int64
	LatestDetectedID        uint64
	LatestDetectedAtUnixMs  int64
}

type httpFlowHighWaterSlotValue struct {
	mu    sync.RWMutex
	value HTTPFlowPipelineHighWater
	valid bool
}

type httpFlowDatabaseIdentityCacheValue struct {
	raw      string
	identity string
}

var (
	httpFlowTimingSlots           [HTTPFlowTimingRegistrySize]httpFlowTimingSlot
	httpFlowHighWaterSlots        [httpFlowHighWaterSlotCount]httpFlowHighWaterSlotValue
	httpFlowDatabaseIdentityCache atomic.Pointer[httpFlowDatabaseIdentityCacheValue]
)

func HTTPFlowDatabaseIdentity(raw string) string {
	if raw == "" {
		return ""
	}
	if cached := httpFlowDatabaseIdentityCache.Load(); cached != nil && cached.raw == raw {
		return cached.identity
	}
	sum := sha256.Sum256([]byte(raw))
	identity := hex.EncodeToString(sum[:8])
	httpFlowDatabaseIdentityCache.Store(&httpFlowDatabaseIdentityCacheValue{raw: raw, identity: identity})
	return identity
}

func RecordHTTPFlowPersisted(flow *schema.HTTPFlow) {
	if flow == nil || flow.ID == 0 || flow.RuntimeTiming == nil {
		return
	}
	timing := flow.RuntimeTiming
	value := HTTPFlowPersistTiming{
		ID:                      uint64(flow.ID),
		DatabaseIdentity:        timing.DatabaseIdentity,
		ProjectGeneration:       timing.ProjectGeneration,
		RequestHijackAtUnixMs:   timing.RequestHijackAtUnixMs,
		ResponseMirrorAtUnixMs:  timing.ResponseMirrorAtUnixMs,
		FlowBuiltAtUnixMs:       timing.FlowBuiltAtUnixMs,
		PersistEnqueuedAtUnixMs: timing.PersistEnqueuedAtUnixMs,
		PersistStartedAtUnixMs:  timing.PersistStartedAtUnixMs,
		PersistedAtUnixMs:       timing.PersistedAtUnixMs,
	}

	slot := &httpFlowTimingSlots[value.ID%HTTPFlowTimingRegistrySize]
	slot.mu.Lock()
	slot.value = value
	slot.valid = true
	slot.mu.Unlock()

	// The MITM table only renders SourceType=mitm. TrafficGuard can deliberately
	// turn a V2 flow into SourceType=scan, which must not inflate its backlog.
	if flow.SourceType != schema.HTTPFlow_SourceType_MITM {
		return
	}
	updateHTTPFlowHighWater(value.DatabaseIdentity, value.ProjectGeneration, func(next *HTTPFlowPipelineHighWater) {
		if value.ID >= next.LatestPersistedID {
			next.LatestPersistedID = value.ID
			next.LatestPersistedAtUnixMs = value.PersistedAtUnixMs
		}
	})
}

func GetHTTPFlowPersistTiming(databaseIdentity string, projectGeneration, id uint64) (HTTPFlowPersistTiming, bool) {
	if id == 0 {
		return HTTPFlowPersistTiming{}, false
	}
	slot := &httpFlowTimingSlots[id%HTTPFlowTimingRegistrySize]
	slot.mu.RLock()
	defer slot.mu.RUnlock()
	if !slot.valid || slot.value.ID != id || slot.value.DatabaseIdentity != databaseIdentity ||
		slot.value.ProjectGeneration != projectGeneration {
		return HTTPFlowPersistTiming{}, false
	}
	return slot.value, true
}

func RecordHTTPFlowChangeDetected(databaseIdentity string, projectGeneration, previousID, id uint64, detectedAt time.Time) {
	if databaseIdentity == "" {
		return
	}
	detectedAtUnixMs := detectedAt.UnixMilli()
	var exactPersistedAtUnixMs int64
	if id > 0 {
		slot := &httpFlowTimingSlots[id%HTTPFlowTimingRegistrySize]
		slot.mu.Lock()
		if slot.valid && slot.value.ID == id && slot.value.DatabaseIdentity == databaseIdentity &&
			slot.value.ProjectGeneration == projectGeneration {
			slot.value.DatabaseChangeDetectedAtUnixMs = detectedAtUnixMs
			exactPersistedAtUnixMs = slot.value.PersistedAtUnixMs
		}
		slot.mu.Unlock()
	}

	updateHTTPFlowHighWater(databaseIdentity, projectGeneration, func(next *HTTPFlowPipelineHighWater) {
		if id < previousID {
			*next = HTTPFlowPipelineHighWater{
				DatabaseIdentity:        databaseIdentity,
				ProjectGeneration:       projectGeneration,
				LatestPersistedID:       id,
				LatestPersistedAtUnixMs: exactPersistedAtUnixMs,
				LatestDetectedID:        id,
				LatestDetectedAtUnixMs:  detectedAtUnixMs,
			}
			return
		}
		if id >= next.LatestDetectedID {
			next.LatestDetectedID = id
			next.LatestDetectedAtUnixMs = detectedAtUnixMs
		}
	})
}

func SnapshotHTTPFlowPipelineHighWater(databaseIdentity string, projectGeneration uint64) HTTPFlowPipelineHighWater {
	slot := &httpFlowHighWaterSlots[httpFlowHighWaterSlot(databaseIdentity, projectGeneration)]
	slot.mu.RLock()
	defer slot.mu.RUnlock()
	if !slot.valid || slot.value.DatabaseIdentity != databaseIdentity || slot.value.ProjectGeneration != projectGeneration {
		return HTTPFlowPipelineHighWater{DatabaseIdentity: databaseIdentity, ProjectGeneration: projectGeneration}
	}
	return slot.value
}

func updateHTTPFlowHighWater(databaseIdentity string, projectGeneration uint64, update func(*HTTPFlowPipelineHighWater)) {
	slot := &httpFlowHighWaterSlots[httpFlowHighWaterSlot(databaseIdentity, projectGeneration)]
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if !slot.valid || slot.value.DatabaseIdentity != databaseIdentity || slot.value.ProjectGeneration != projectGeneration {
		slot.value = HTTPFlowPipelineHighWater{
			DatabaseIdentity:  databaseIdentity,
			ProjectGeneration: projectGeneration,
		}
		slot.valid = true
	}
	update(&slot.value)
}

func httpFlowHighWaterSlot(databaseIdentity string, projectGeneration uint64) uint32 {
	// FNV-1a is sufficient here: DatabaseIdentity is already a SHA-256-derived
	// opaque value and collisions only evict diagnostics, never HTTP flow data.
	var hash uint32 = 2166136261
	for index := 0; index < len(databaseIdentity); index++ {
		hash ^= uint32(databaseIdentity[index])
		hash *= 16777619
	}
	for shift := uint(0); shift < 64; shift += 8 {
		hash ^= uint32(byte(projectGeneration >> shift))
		hash *= 16777619
	}
	return hash % httpFlowHighWaterSlotCount
}

// resetHTTPFlowObservabilityForTest resets the fixed registry without making
// production code expose a mutation API.
func resetHTTPFlowObservabilityForTest() {
	for index := range httpFlowTimingSlots {
		slot := &httpFlowTimingSlots[index]
		slot.mu.Lock()
		slot.value = HTTPFlowPersistTiming{}
		slot.valid = false
		slot.mu.Unlock()
	}
	for index := range httpFlowHighWaterSlots {
		slot := &httpFlowHighWaterSlots[index]
		slot.mu.Lock()
		slot.value = HTTPFlowPipelineHighWater{}
		slot.valid = false
		slot.mu.Unlock()
	}
	httpFlowDatabaseIdentityCache.Store(nil)
}
