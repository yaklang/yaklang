package state

import (
	"encoding/binary"
	"hash/fnv"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

const defaultShards = 64

// Config bounds every high-cardinality state class. Limits are mandatory so a
// zero-valued Config is rejected; DefaultConfig provides production defaults.
type Config struct {
	Shards             int
	MaxFlows           int
	MaxFlowBitsPerFlow int
	MaxFlowIntsPerFlow int
	MaxXBits           int
	MaxThresholds      int
	MaxNameLength      int
	FlowIdleTimeout    time.Duration
}

func DefaultConfig() Config {
	return Config{
		Shards:             defaultShards,
		MaxFlows:           100_000,
		MaxFlowBitsPerFlow: 256,
		MaxFlowIntsPerFlow: 256,
		MaxXBits:           100_000,
		MaxThresholds:      100_000,
		MaxNameLength:      256,
		FlowIdleTimeout:    5 * time.Minute,
	}
}

// Store is a sharded concurrent state store. The lifecycle lock is shared by
// operations and exclusive to Close, so Close waits for active operations.
type Store struct {
	lifecycle sync.RWMutex
	closed    bool
	clock     Clock
	cfg       Config

	flows      []flowShard
	xbits      []xbitShard
	thresholds []thresholdShard

	flowCount      atomic.Int64
	xbitCount      atomic.Int64
	thresholdCount atomic.Int64
	rejected       atomic.Uint64
	cleaned        atomic.Uint64
}

type flowShard struct {
	sync.Mutex
	items map[FlowKey]*flowEntry
}

type xbitShard struct {
	sync.Mutex
	items map[XBitKey]xbitEntry
}

type thresholdShard struct {
	sync.Mutex
	items map[ThresholdKey]thresholdEntry
}

// Stats is an instantaneous, concurrency-safe store summary.
type Stats struct {
	Flows      int
	FlowBits   int
	FlowInts   int
	XBits      int
	Thresholds int
	Rejected   uint64
	Cleaned    uint64
	Closed     bool
}

func NewStore(cfg Config, clock Clock) (*Store, error) {
	if cfg.Shards <= 0 || cfg.MaxFlows <= 0 || cfg.MaxFlowBitsPerFlow <= 0 ||
		cfg.MaxFlowIntsPerFlow <= 0 || cfg.MaxXBits <= 0 || cfg.MaxThresholds <= 0 ||
		cfg.MaxNameLength <= 0 || cfg.FlowIdleTimeout < 0 {
		return nil, ErrInvalidArgument
	}
	if clock == nil {
		clock = wallClock{}
	}
	s := &Store{
		clock:      clock,
		cfg:        cfg,
		flows:      make([]flowShard, cfg.Shards),
		xbits:      make([]xbitShard, cfg.Shards),
		thresholds: make([]thresholdShard, cfg.Shards),
	}
	for i := range s.flows {
		s.flows[i].items = make(map[FlowKey]*flowEntry)
		s.xbits[i].items = make(map[XBitKey]xbitEntry)
		s.thresholds[i].items = make(map[ThresholdKey]thresholdEntry)
	}
	return s, nil
}

func NewDefaultStore(clock Clock) *Store {
	s, _ := NewStore(DefaultConfig(), clock)
	return s
}

// Close rejects future operations and releases all retained state. It is
// idempotent and waits for active operations to return.
func (s *Store) Close() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	for i := range s.flows {
		s.flows[i].items = make(map[FlowKey]*flowEntry)
		s.xbits[i].items = make(map[XBitKey]xbitEntry)
		s.thresholds[i].items = make(map[ThresholdKey]thresholdEntry)
	}
	s.flowCount.Store(0)
	s.xbitCount.Store(0)
	s.thresholdCount.Store(0)
	return nil
}

func (s *Store) begin() error {
	s.lifecycle.RLock()
	if s.closed {
		s.lifecycle.RUnlock()
		return ErrClosed
	}
	return nil
}

func (s *Store) end() { s.lifecycle.RUnlock() }

// Cleanup removes expired xbits, completed threshold windows, and idle flows.
// now is capture time; a zero value uses Clock.Now. Backward time never expires
// state whose latest observed capture time is newer than now.
func (s *Store) Cleanup(now time.Time) (int, error) {
	if err := s.begin(); err != nil {
		return 0, err
	}
	defer s.end()
	if now.IsZero() {
		now = s.clock.Now()
	}
	removed := 0
	if s.cfg.FlowIdleTimeout > 0 {
		for i := range s.flows {
			sh := &s.flows[i]
			sh.Lock()
			for key, entry := range sh.items {
				entry.mu.Lock()
				idle := !now.Before(entry.state.updatedAt) && now.Sub(entry.state.updatedAt) >= s.cfg.FlowIdleTimeout
				entry.mu.Unlock()
				if idle {
					delete(sh.items, key)
					s.flowCount.Add(-1)
					removed++
				}
			}
			sh.Unlock()
		}
	}
	for i := range s.xbits {
		sh := &s.xbits[i]
		sh.Lock()
		for key, entry := range sh.items {
			if !entry.expiresAt.IsZero() && !now.Before(entry.lastTime) && !now.Before(entry.expiresAt) {
				delete(sh.items, key)
				s.xbitCount.Add(-1)
				removed++
			}
		}
		sh.Unlock()
	}
	for i := range s.thresholds {
		sh := &s.thresholds[i]
		sh.Lock()
		for key, entry := range sh.items {
			if !now.Before(entry.lastTime) && !now.Before(entry.windowStart.Add(entry.spec.Window)) {
				delete(sh.items, key)
				s.thresholdCount.Add(-1)
				removed++
			}
		}
		sh.Unlock()
	}
	s.cleaned.Add(uint64(removed))
	return removed, nil
}

func (s *Store) Stats() Stats {
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	stats := Stats{
		Flows:      int(s.flowCount.Load()),
		XBits:      int(s.xbitCount.Load()),
		Thresholds: int(s.thresholdCount.Load()),
		Rejected:   s.rejected.Load(),
		Cleaned:    s.cleaned.Load(),
		Closed:     s.closed,
	}
	if s.closed {
		return stats
	}
	for i := range s.flows {
		sh := &s.flows[i]
		sh.Lock()
		for _, entry := range sh.items {
			entry.mu.Lock()
			stats.FlowBits += len(entry.state.bits)
			stats.FlowInts += len(entry.state.ints)
			entry.mu.Unlock()
		}
		sh.Unlock()
	}
	return stats
}

func (s *Store) reserve(counter *atomic.Int64, limit int) bool {
	for {
		current := counter.Load()
		if current >= int64(limit) {
			s.rejected.Add(1)
			return false
		}
		if counter.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (s *Store) validName(name string) error {
	if name == "" {
		return ErrInvalidName
	}
	if len(name) > s.cfg.MaxNameLength {
		return ErrNameTooLong
	}
	return nil
}

func validFlowKey(key FlowKey) bool {
	return key.A.IsValid() && key.B.IsValid() && compareAddrPort(key.A, key.B) <= 0
}

func validHostKey(key HostKey) bool { return key.Addr.IsValid() }

func validPairKey(key PairKey) bool {
	return key.A.IsValid() && key.B.IsValid() && key.A.Compare(key.B) <= 0
}

func hashFlowKey(key FlowKey) uint64 {
	h := fnv.New64a()
	writeUint64(h, key.Epoch)
	writeDomain(h, key.Domain)
	writeUint64(h, key.Generation)
	_, _ = h.Write([]byte{key.Protocol})
	writeAddrPort(h, key.A)
	writeAddrPort(h, key.B)
	return h.Sum64()
}

func writeDomain(h interface{ Write([]byte) (int, error) }, d CaptureDomain) {
	_, _ = h.Write([]byte(d.Sensor))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(d.Interface))
	_, _ = h.Write([]byte{0})
	writeUint64(h, d.ID)
}

func writeUint64(h interface{ Write([]byte) (int, error) }, value uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	_, _ = h.Write(buf[:])
}

func writeAddr(h interface{ Write([]byte) (int, error) }, addr netip.Addr) {
	if addr.Is4() {
		value := addr.As4()
		_, _ = h.Write(value[:])
		return
	}
	value := addr.As16()
	_, _ = h.Write(value[:])
}

func writeAddrPort(h interface{ Write([]byte) (int, error) }, addr netip.AddrPort) {
	writeAddr(h, addr.Addr())
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], addr.Port())
	_, _ = h.Write(buf[:])
}
