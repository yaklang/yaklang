package state

import (
	"hash/fnv"
	"time"
)

// ThresholdMode defines fixed-window alert behavior.
type ThresholdMode uint8

const (
	ThresholdLimit     ThresholdMode = iota + 1 // alert for counts 1..N
	ThresholdThreshold                          // alert on every Nth event
	ThresholdBoth                               // alert once, exactly at N
	DetectionFilter                             // alert after N events

	// ThresholdEvery is an alias emphasizing ThresholdThreshold's behavior.
	ThresholdEvery = ThresholdThreshold
)

// ThresholdSpec is immutable for a ThresholdKey within one epoch.
type ThresholdSpec struct {
	Mode   ThresholdMode
	Count  uint64
	Window time.Duration
}

// ThresholdScope selects the owner encoded by ThresholdKey.
type ThresholdScope uint8

const (
	ThresholdGlobal ThresholdScope = iota + 1
	ThresholdFlow
	ThresholdHost
	ThresholdPair
)

// ThresholdKey identifies one rule counter and tracking subject. RuleID is a
// caller-owned stable identifier (for example SID plus an internal rule ID).
type ThresholdKey struct {
	Epoch  uint64
	Domain CaptureDomain
	RuleID uint64
	Scope  ThresholdScope
	Flow   FlowKey
	Host   HostKey
	Pair   PairKey
}

func GlobalThresholdKey(epoch uint64, domain CaptureDomain, ruleID uint64) ThresholdKey {
	return ThresholdKey{Epoch: epoch, Domain: domain, RuleID: ruleID, Scope: ThresholdGlobal}
}

func FlowThresholdKey(ruleID uint64, flow FlowKey) ThresholdKey {
	return ThresholdKey{Epoch: flow.Epoch, Domain: flow.Domain, RuleID: ruleID, Scope: ThresholdFlow, Flow: flow}
}

func HostThresholdKey(ruleID uint64, host HostKey) ThresholdKey {
	return ThresholdKey{Epoch: host.Epoch, Domain: host.Domain, RuleID: ruleID, Scope: ThresholdHost, Host: host}
}

func PairThresholdKey(ruleID uint64, pair PairKey) ThresholdKey {
	return ThresholdKey{Epoch: pair.Epoch, Domain: pair.Domain, RuleID: ruleID, Scope: ThresholdPair, Pair: pair}
}

// ThresholdResult describes the event after it is counted.
type ThresholdResult struct {
	Alert       bool
	Count       uint64
	WindowStart time.Time
}

type thresholdEntry struct {
	spec        ThresholdSpec
	count       uint64
	windowStart time.Time
	lastTime    time.Time
}

// CheckThreshold counts one event using Clock.Now.
func (s *Store) CheckThreshold(key ThresholdKey, spec ThresholdSpec) (ThresholdResult, error) {
	return s.CheckThresholdAt(key, s.clock.Now(), spec)
}

// CheckThresholdAt counts one event at an explicit capture timestamp. Windows
// are half-open: [WindowStart, WindowStart+Window), so an event exactly at the
// end starts a new window. Capture-time rollback is clamped to the last time.
func (s *Store) CheckThresholdAt(key ThresholdKey, now time.Time, spec ThresholdSpec) (ThresholdResult, error) {
	if err := validateThresholdSpec(spec); err != nil {
		return ThresholdResult{}, err
	}
	if !validThresholdKey(key) {
		return ThresholdResult{}, ErrInvalidKey
	}
	if err := s.begin(); err != nil {
		return ThresholdResult{}, err
	}
	defer s.end()
	if s.thresholdCount.Load() >= int64(s.cfg.MaxThresholds) {
		s.cleanupExpiredThresholdsAt(now)
	}
	sh := &s.thresholds[hashThresholdKey(key)%uint64(len(s.thresholds))]
	sh.Lock()
	defer sh.Unlock()
	entry, exists := sh.items[key]
	if !exists {
		if !s.reserve(&s.thresholdCount, s.cfg.MaxThresholds) {
			return ThresholdResult{}, ErrCapacity
		}
		entry = thresholdEntry{spec: spec, windowStart: now, lastTime: now}
	} else {
		if entry.spec != spec {
			return ThresholdResult{}, ErrSpecMismatch
		}
		if now.Before(entry.lastTime) {
			now = entry.lastTime
		}
		if !now.Before(entry.windowStart.Add(spec.Window)) {
			entry.count = 0
			entry.windowStart = now
		}
		entry.lastTime = now
	}
	entry.count++
	sh.items[key] = entry
	return ThresholdResult{
		Alert:       thresholdAlert(spec.Mode, entry.count, spec.Count),
		Count:       entry.count,
		WindowStart: entry.windowStart,
	}, nil
}

func (s *Store) cleanupExpiredThresholdsAt(now time.Time) {
	for index := range s.thresholds {
		shard := &s.thresholds[index]
		shard.Lock()
		for key, entry := range shard.items {
			if !now.Before(entry.lastTime) && !now.Before(entry.windowStart.Add(entry.spec.Window)) {
				delete(shard.items, key)
				s.thresholdCount.Add(-1)
				s.cleaned.Add(1)
			}
		}
		shard.Unlock()
	}
}

func validateThresholdSpec(spec ThresholdSpec) error {
	if spec.Count == 0 || spec.Window <= 0 {
		return ErrInvalidArgument
	}
	switch spec.Mode {
	case ThresholdLimit, ThresholdEvery, ThresholdBoth, DetectionFilter:
		return nil
	default:
		return ErrInvalidArgument
	}
}

func thresholdAlert(mode ThresholdMode, count, target uint64) bool {
	switch mode {
	case ThresholdLimit:
		return count <= target
	case ThresholdEvery:
		return count%target == 0
	case ThresholdBoth:
		return count == target
	case DetectionFilter:
		return count > target
	default:
		return false
	}
}

func validThresholdKey(key ThresholdKey) bool {
	switch key.Scope {
	case ThresholdGlobal:
		return key.Flow == (FlowKey{}) && key.Host == (HostKey{}) && key.Pair == (PairKey{})
	case ThresholdFlow:
		return validFlowKey(key.Flow) && key.Epoch == key.Flow.Epoch && key.Domain == key.Flow.Domain && key.Host == (HostKey{}) && key.Pair == (PairKey{})
	case ThresholdHost:
		return validHostKey(key.Host) && key.Epoch == key.Host.Epoch && key.Domain == key.Host.Domain && key.Flow == (FlowKey{}) && key.Pair == (PairKey{})
	case ThresholdPair:
		return validPairKey(key.Pair) && key.Epoch == key.Pair.Epoch && key.Domain == key.Pair.Domain && key.Flow == (FlowKey{}) && key.Host == (HostKey{})
	default:
		return false
	}
}

func hashThresholdKey(key ThresholdKey) uint64 {
	h := fnv.New64a()
	writeUint64(h, key.Epoch)
	writeDomain(h, key.Domain)
	writeUint64(h, key.RuleID)
	_, _ = h.Write([]byte{byte(key.Scope)})
	switch key.Scope {
	case ThresholdFlow:
		writeUint64(h, hashFlowKey(key.Flow))
	case ThresholdHost:
		writeAddr(h, key.Host.Addr)
	case ThresholdPair:
		writeAddr(h, key.Pair.A)
		writeAddr(h, key.Pair.B)
	}
	return h.Sum64()
}
