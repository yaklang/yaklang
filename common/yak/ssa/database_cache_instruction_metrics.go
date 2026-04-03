package ssa

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"go.uber.org/atomic"
)

type instructionCacheMetrics struct {
	reloadTotal             *atomic.Uint64
	reloadTimeTotal         *atomic.Duration
	saveTotal               *atomic.Uint64
	saveTimeTotal           *atomic.Duration
	saveCloseTotal          *atomic.Uint64
	saveCloseTimeTotal      *atomic.Duration
	saveTTLTotal            *atomic.Uint64
	saveTTLTimeTotal        *atomic.Duration
	saveMaxTotal            *atomic.Uint64
	saveMaxTimeTotal        *atomic.Duration
	writebackTotal          *atomic.Uint64
	writebackTimeTotal      *atomic.Duration
	writebackCloseTotal     *atomic.Uint64
	writebackCloseTimeTotal *atomic.Duration
	writebackTTLTotal       *atomic.Uint64
	writebackTTLTimeTotal   *atomic.Duration
	writebackMaxTotal       *atomic.Uint64
	writebackMaxTimeTotal   *atomic.Duration
	ttlEvictTotal           *atomic.Uint64
	maxEvictTotal           *atomic.Uint64
	sourceSaveAttempts      *atomic.Uint64
	sourceSaveUnique        *atomic.Uint64

	mu                     sync.Mutex
	reloadByOpcode         map[Opcode]uint64
	saveByOpcode           map[Opcode]uint64
	saveCloseByOpcode      map[Opcode]uint64
	saveTTLByOpcode        map[Opcode]uint64
	saveMaxByOpcode        map[Opcode]uint64
	writebackByOpcode      map[Opcode]uint64
	writebackCloseByOpcode map[Opcode]uint64
	writebackTTLByOpcode   map[Opcode]uint64
	writebackMaxByOpcode   map[Opcode]uint64
}

func newInstructionCacheMetrics() *instructionCacheMetrics {
	return &instructionCacheMetrics{
		reloadTotal:             atomic.NewUint64(0),
		reloadTimeTotal:         atomic.NewDuration(0),
		saveTotal:               atomic.NewUint64(0),
		saveTimeTotal:           atomic.NewDuration(0),
		saveCloseTotal:          atomic.NewUint64(0),
		saveCloseTimeTotal:      atomic.NewDuration(0),
		saveTTLTotal:            atomic.NewUint64(0),
		saveTTLTimeTotal:        atomic.NewDuration(0),
		saveMaxTotal:            atomic.NewUint64(0),
		saveMaxTimeTotal:        atomic.NewDuration(0),
		writebackTotal:          atomic.NewUint64(0),
		writebackTimeTotal:      atomic.NewDuration(0),
		writebackCloseTotal:     atomic.NewUint64(0),
		writebackCloseTimeTotal: atomic.NewDuration(0),
		writebackTTLTotal:       atomic.NewUint64(0),
		writebackTTLTimeTotal:   atomic.NewDuration(0),
		writebackMaxTotal:       atomic.NewUint64(0),
		writebackMaxTimeTotal:   atomic.NewDuration(0),
		ttlEvictTotal:           atomic.NewUint64(0),
		maxEvictTotal:           atomic.NewUint64(0),
		sourceSaveAttempts:      atomic.NewUint64(0),
		sourceSaveUnique:        atomic.NewUint64(0),
		reloadByOpcode:          make(map[Opcode]uint64),
		saveByOpcode:            make(map[Opcode]uint64),
		saveCloseByOpcode:       make(map[Opcode]uint64),
		saveTTLByOpcode:         make(map[Opcode]uint64),
		saveMaxByOpcode:         make(map[Opcode]uint64),
		writebackByOpcode:       make(map[Opcode]uint64),
		writebackCloseByOpcode:  make(map[Opcode]uint64),
		writebackTTLByOpcode:    make(map[Opcode]uint64),
		writebackMaxByOpcode:    make(map[Opcode]uint64),
	}
}

func (m *instructionCacheMetrics) RecordReload(op Opcode, cost time.Duration) {
	if m == nil {
		return
	}
	m.reloadTotal.Inc()
	m.reloadTimeTotal.Add(cost)
	m.mu.Lock()
	m.reloadByOpcode[op]++
	m.mu.Unlock()
}

func (m *instructionCacheMetrics) RecordWriteback(reason utils.EvictionReason, op Opcode, cost time.Duration) {
	if m == nil {
		return
	}
	m.writebackTotal.Inc()
	m.writebackTimeTotal.Add(cost)
	m.mu.Lock()
	m.writebackByOpcode[op]++
	switch reason {
	case utils.EvictionReasonDeleted:
		m.writebackCloseTotal.Inc()
		m.writebackCloseTimeTotal.Add(cost)
		m.writebackCloseByOpcode[op]++
	case utils.EvictionReasonExpired:
		m.writebackTTLTotal.Inc()
		m.writebackTTLTimeTotal.Add(cost)
		m.writebackTTLByOpcode[op]++
	case utils.EvictionReasonCapacityReached:
		m.writebackMaxTotal.Inc()
		m.writebackMaxTimeTotal.Add(cost)
		m.writebackMaxByOpcode[op]++
	}
	m.mu.Unlock()
}

func (m *instructionCacheMetrics) RecordSave(reason utils.EvictionReason, op Opcode, cost time.Duration) {
	if m == nil {
		return
	}
	m.saveTotal.Inc()
	m.saveTimeTotal.Add(cost)
	m.mu.Lock()
	m.saveByOpcode[op]++
	switch reason {
	case utils.EvictionReasonDeleted:
		m.saveCloseTotal.Inc()
		m.saveCloseTimeTotal.Add(cost)
		m.saveCloseByOpcode[op]++
	case utils.EvictionReasonExpired:
		m.saveTTLTotal.Inc()
		m.saveTTLTimeTotal.Add(cost)
		m.saveTTLByOpcode[op]++
	case utils.EvictionReasonCapacityReached:
		m.saveMaxTotal.Inc()
		m.saveMaxTimeTotal.Add(cost)
		m.saveMaxByOpcode[op]++
	}
	m.mu.Unlock()
}

func (m *instructionCacheMetrics) RecordSourceSave(attempts, unique int) {
	if m == nil {
		return
	}
	if attempts > 0 {
		m.sourceSaveAttempts.Add(uint64(attempts))
	}
	if unique > 0 {
		m.sourceSaveUnique.Add(uint64(unique))
	}
}

func (m *instructionCacheMetrics) RecordEvict(reason utils.EvictionReason, _ Opcode) {
	if m == nil {
		return
	}
	switch reason {
	case utils.EvictionReasonExpired:
		m.ttlEvictTotal.Inc()
	case utils.EvictionReasonCapacityReached:
		m.maxEvictTotal.Inc()
	}
}

func (m *instructionCacheMetrics) Dump(programName string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	reloadTop := topOpcodeStats(m.reloadByOpcode, 8)
	saveTop := topOpcodeStats(m.saveByOpcode, 8)
	saveCloseTop := topOpcodeStats(m.saveCloseByOpcode, 8)
	saveTTLTop := topOpcodeStats(m.saveTTLByOpcode, 8)
	saveMaxTop := topOpcodeStats(m.saveMaxByOpcode, 8)
	writebackTop := topOpcodeStats(m.writebackByOpcode, 8)
	writebackCloseTop := topOpcodeStats(m.writebackCloseByOpcode, 8)
	writebackTTLTop := topOpcodeStats(m.writebackTTLByOpcode, 8)
	writebackMaxTop := topOpcodeStats(m.writebackMaxByOpcode, 8)
	m.mu.Unlock()
	log.Debugf(
		"[ssa-ir-cache-summary] program=%s reload=%d reload_time=%s save=%d save_time=%s save_close=%d save_close_time=%s save_ttl=%d save_ttl_time=%s save_max=%d save_max_time=%s writeback=%d writeback_time=%s writeback_close=%d writeback_close_time=%s writeback_ttl=%d writeback_ttl_time=%s writeback_max=%d writeback_max_time=%s ttl_evict=%d max_evict=%d source_save_attempts=%d source_save_unique=%d reload_top=[%s] save_top=[%s] save_close_top=[%s] save_ttl_top=[%s] save_max_top=[%s] writeback_top=[%s] writeback_close_top=[%s] writeback_ttl_top=[%s] writeback_max_top=[%s]",
		programName,
		m.reloadTotal.Load(),
		m.reloadTimeTotal.Load(),
		m.saveTotal.Load(),
		m.saveTimeTotal.Load(),
		m.saveCloseTotal.Load(),
		m.saveCloseTimeTotal.Load(),
		m.saveTTLTotal.Load(),
		m.saveTTLTimeTotal.Load(),
		m.saveMaxTotal.Load(),
		m.saveMaxTimeTotal.Load(),
		m.writebackTotal.Load(),
		m.writebackTimeTotal.Load(),
		m.writebackCloseTotal.Load(),
		m.writebackCloseTimeTotal.Load(),
		m.writebackTTLTotal.Load(),
		m.writebackTTLTimeTotal.Load(),
		m.writebackMaxTotal.Load(),
		m.writebackMaxTimeTotal.Load(),
		m.ttlEvictTotal.Load(),
		m.maxEvictTotal.Load(),
		m.sourceSaveAttempts.Load(),
		m.sourceSaveUnique.Load(),
		reloadTop,
		saveTop,
		saveCloseTop,
		saveTTLTop,
		saveMaxTop,
		writebackTop,
		writebackCloseTop,
		writebackTTLTop,
		writebackMaxTop,
	)
}

func evictionReasonName(reason utils.EvictionReason) string {
	switch reason {
	case utils.EvictionReasonDeleted:
		return "deleted"
	case utils.EvictionReasonCapacityReached:
		return "capacity"
	case utils.EvictionReasonExpired:
		return "expired"
	default:
		return "unknown"
	}
}

func topOpcodeStats(values map[Opcode]uint64, topN int) string {
	if len(values) == 0 || topN <= 0 {
		return ""
	}
	type opcodeStat struct {
		op    Opcode
		count uint64
	}
	items := make([]opcodeStat, 0, len(values))
	for op, count := range values {
		items = append(items, opcodeStat{op: op, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].count > items[j].count
	})
	if len(items) > topN {
		items = items[:topN]
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, fmt.Sprintf("%s=%d", item.op.String(), item.count))
	}
	return strings.Join(result, " ")
}
