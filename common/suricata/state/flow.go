package state

import (
	"math"
	"sync"
	"time"
)

// IntCompare is a flowint comparison operator.
type IntCompare uint8

const (
	IntEqual IntCompare = iota + 1
	IntNotEqual
	IntLess
	IntLessOrEqual
	IntGreater
	IntGreaterOrEqual
)

// FlowSnapshot is a detached copy safe to retain after a transaction returns.
type FlowSnapshot struct {
	Key       FlowKey
	CreatedAt time.Time
	UpdatedAt time.Time
	Bits      map[string]bool
	Ints      map[string]int64
}

// FlowView provides read-only predicate access. It is valid only during the
// callback that received it.
type FlowView struct{ state *FlowState }

func (v FlowView) TestBit(name string) (bool, error)       { return v.state.TestBit(name) }
func (v FlowView) GetInt(name string) (int64, bool, error) { return v.state.GetInt(name) }
func (v FlowView) CompareInt(name string, op IntCompare, rhs int64) (bool, error) {
	return v.state.CompareInt(name, op, rhs)
}
func (v FlowView) Snapshot() FlowSnapshot { return v.state.Snapshot() }

// FlowState is the mutable transaction value passed to WithFlow and effects.
// It must not be retained or used after the callback returns.
type FlowState struct {
	key          FlowKey
	createdAt    time.Time
	updatedAt    time.Time
	bits         map[string]bool
	ints         map[string]int64
	maxBits      int
	maxInts      int
	maxNameBytes int
}

type flowEntry struct {
	mu    sync.Mutex
	state *FlowState
}

func newFlowState(key FlowKey, now time.Time, cfg Config) *FlowState {
	return &FlowState{
		key:          key,
		createdAt:    now,
		updatedAt:    now,
		bits:         make(map[string]bool),
		ints:         make(map[string]int64),
		maxBits:      cfg.MaxFlowBitsPerFlow,
		maxInts:      cfg.MaxFlowIntsPerFlow,
		maxNameBytes: cfg.MaxNameLength,
	}
}

func (f *FlowState) clone() *FlowState {
	clone := *f
	clone.bits = make(map[string]bool, len(f.bits))
	for name, value := range f.bits {
		clone.bits[name] = value
	}
	clone.ints = make(map[string]int64, len(f.ints))
	for name, value := range f.ints {
		clone.ints[name] = value
	}
	return &clone
}

func (f *FlowState) validateName(name string) error {
	if name == "" {
		return ErrInvalidName
	}
	if len(name) > f.maxNameBytes {
		return ErrNameTooLong
	}
	return nil
}

func (f *FlowState) Key() FlowKey         { return f.key }
func (f *FlowState) CreatedAt() time.Time { return f.createdAt }
func (f *FlowState) UpdatedAt() time.Time { return f.updatedAt }

func (f *FlowState) Snapshot() FlowSnapshot {
	bits := make(map[string]bool, len(f.bits))
	for name, value := range f.bits {
		bits[name] = value
	}
	ints := make(map[string]int64, len(f.ints))
	for name, value := range f.ints {
		ints[name] = value
	}
	return FlowSnapshot{Key: f.key, CreatedAt: f.createdAt, UpdatedAt: f.updatedAt, Bits: bits, Ints: ints}
}

func (f *FlowState) SetBit(name string) error {
	if err := f.validateName(name); err != nil {
		return err
	}
	if !f.bits[name] && len(f.bits) >= f.maxBits {
		return ErrPerFlowLimit
	}
	f.bits[name] = true
	return nil
}

func (f *FlowState) UnsetBit(name string) error {
	if err := f.validateName(name); err != nil {
		return err
	}
	delete(f.bits, name)
	return nil
}

func (f *FlowState) ToggleBit(name string) (bool, error) {
	if err := f.validateName(name); err != nil {
		return false, err
	}
	if f.bits[name] {
		delete(f.bits, name)
		return false, nil
	}
	if len(f.bits) >= f.maxBits {
		return false, ErrPerFlowLimit
	}
	f.bits[name] = true
	return true, nil
}

func (f *FlowState) TestBit(name string) (bool, error) {
	if err := f.validateName(name); err != nil {
		return false, err
	}
	return f.bits[name], nil
}

func (f *FlowState) GetInt(name string) (int64, bool, error) {
	if err := f.validateName(name); err != nil {
		return 0, false, err
	}
	value, ok := f.ints[name]
	return value, ok, nil
}

func (f *FlowState) SetInt(name string, value int64) error {
	if err := f.validateName(name); err != nil {
		return err
	}
	if _, exists := f.ints[name]; !exists && len(f.ints) >= f.maxInts {
		return ErrPerFlowLimit
	}
	f.ints[name] = value
	return nil
}

func (f *FlowState) AddInt(name string, delta int64) (int64, error) {
	if err := f.validateName(name); err != nil {
		return 0, err
	}
	current, exists := f.ints[name]
	if !exists && len(f.ints) >= f.maxInts {
		return 0, ErrPerFlowLimit
	}
	if (delta > 0 && current > math.MaxInt64-delta) || (delta < 0 && current < math.MinInt64-delta) {
		return current, ErrOverflow
	}
	current += delta
	f.ints[name] = current
	return current, nil
}

func (f *FlowState) CompareInt(name string, op IntCompare, rhs int64) (bool, error) {
	value, exists, err := f.GetInt(name)
	if err != nil || !exists {
		return false, err
	}
	switch op {
	case IntEqual:
		return value == rhs, nil
	case IntNotEqual:
		return value != rhs, nil
	case IntLess:
		return value < rhs, nil
	case IntLessOrEqual:
		return value <= rhs, nil
	case IntGreater:
		return value > rhs, nil
	case IntGreaterOrEqual:
		return value >= rhs, nil
	default:
		return false, ErrInvalidArgument
	}
}

// WithFlow serializes a copy-on-write transaction for key using Clock.Now. A
// nil callback or callback error rolls back all changes.
func (s *Store) WithFlow(key FlowKey, fn func(*FlowState) error) error {
	return s.WithFlowAt(key, s.clock.Now(), fn)
}

// WithFlowAt is WithFlow with an explicit capture timestamp. Different flow
// keys can run in parallel. Backward timestamps do not move UpdatedAt backward.
func (s *Store) WithFlowAt(key FlowKey, now time.Time, fn func(*FlowState) error) error {
	if fn == nil {
		return ErrInvalidArgument
	}
	if !validFlowKey(key) {
		return ErrInvalidKey
	}
	if err := s.begin(); err != nil {
		return err
	}
	defer s.end()
	entry, err := s.flowEntry(key, true, now)
	if err != nil {
		return err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	working := entry.state.clone()
	if err := fn(working); err != nil {
		return err
	}
	if now.After(working.updatedAt) {
		working.updatedAt = now
	}
	entry.state = working
	return nil
}

// Evaluate atomically runs predicate then effects under the same flow lock using
// Clock.Now. Effects execute only for a true predicate and commit together.
func (s *Store) Evaluate(key FlowKey, predicate func(FlowView) (bool, error), effects ...func(*FlowState) error) (bool, error) {
	return s.EvaluateAt(key, s.clock.Now(), predicate, effects...)
}

// EvaluateAt is Evaluate with an explicit capture timestamp.
func (s *Store) EvaluateAt(key FlowKey, now time.Time, predicate func(FlowView) (bool, error), effects ...func(*FlowState) error) (bool, error) {
	if predicate == nil {
		return false, ErrInvalidArgument
	}
	if !validFlowKey(key) {
		return false, ErrInvalidKey
	}
	if err := s.begin(); err != nil {
		return false, err
	}
	defer s.end()
	entry, err := s.flowEntry(key, true, now)
	if err != nil {
		return false, err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	matched, err := predicate(FlowView{state: entry.state})
	if err != nil || !matched {
		return matched, err
	}
	working := entry.state.clone()
	for _, effect := range effects {
		if effect == nil {
			return false, ErrInvalidArgument
		}
		if err := effect(working); err != nil {
			return false, err
		}
	}
	if now.After(working.updatedAt) {
		working.updatedAt = now
	}
	entry.state = working
	return true, nil
}

func (s *Store) flowEntry(key FlowKey, create bool, now time.Time) (*flowEntry, error) {
	sh := &s.flows[hashFlowKey(key)%uint64(len(s.flows))]
	sh.Lock()
	defer sh.Unlock()
	if entry := sh.items[key]; entry != nil {
		return entry, nil
	}
	if !create {
		return nil, nil
	}
	if !s.reserve(&s.flowCount, s.cfg.MaxFlows) {
		return nil, ErrCapacity
	}
	entry := &flowEntry{state: newFlowState(key, now, s.cfg)}
	sh.items[key] = entry
	return entry, nil
}

func (s *Store) DeleteFlow(key FlowKey) (bool, error) {
	if !validFlowKey(key) {
		return false, ErrInvalidKey
	}
	if err := s.begin(); err != nil {
		return false, err
	}
	defer s.end()
	sh := &s.flows[hashFlowKey(key)%uint64(len(s.flows))]
	sh.Lock()
	defer sh.Unlock()
	entry := sh.items[key]
	if entry == nil {
		return false, nil
	}
	entry.mu.Lock()
	delete(sh.items, key)
	entry.mu.Unlock()
	s.flowCount.Add(-1)
	return true, nil
}

func (s *Store) SnapshotFlow(key FlowKey) (FlowSnapshot, bool, error) {
	if !validFlowKey(key) {
		return FlowSnapshot{}, false, ErrInvalidKey
	}
	if err := s.begin(); err != nil {
		return FlowSnapshot{}, false, err
	}
	defer s.end()
	entry, _ := s.flowEntry(key, false, time.Time{})
	if entry == nil {
		return FlowSnapshot{}, false, nil
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.state.Snapshot(), true, nil
}

func (s *Store) SetFlowBit(key FlowKey, name string) error {
	return s.WithFlow(key, func(flow *FlowState) error { return flow.SetBit(name) })
}

func (s *Store) UnsetFlowBit(key FlowKey, name string) error {
	return s.WithFlow(key, func(flow *FlowState) error { return flow.UnsetBit(name) })
}

func (s *Store) ToggleFlowBit(key FlowKey, name string) (bool, error) {
	var value bool
	err := s.WithFlow(key, func(flow *FlowState) error {
		var err error
		value, err = flow.ToggleBit(name)
		return err
	})
	return value, err
}

func (s *Store) TestFlowBit(key FlowKey, name string) (bool, error) {
	snapshot, ok, err := s.SnapshotFlow(key)
	if err != nil || !ok {
		return false, err
	}
	if err := s.validName(name); err != nil {
		return false, err
	}
	return snapshot.Bits[name], nil
}

func (s *Store) GetFlowInt(key FlowKey, name string) (int64, bool, error) {
	snapshot, ok, err := s.SnapshotFlow(key)
	if err != nil || !ok {
		return 0, false, err
	}
	if err := s.validName(name); err != nil {
		return 0, false, err
	}
	value, exists := snapshot.Ints[name]
	return value, exists, nil
}

func (s *Store) SetFlowInt(key FlowKey, name string, value int64) error {
	return s.WithFlow(key, func(flow *FlowState) error { return flow.SetInt(name, value) })
}

func (s *Store) AddFlowInt(key FlowKey, name string, delta int64) (int64, error) {
	var value int64
	err := s.WithFlow(key, func(flow *FlowState) error {
		var err error
		value, err = flow.AddInt(name, delta)
		return err
	})
	return value, err
}

func (s *Store) CompareFlowInt(key FlowKey, name string, op IntCompare, rhs int64) (bool, error) {
	snapshot, ok, err := s.SnapshotFlow(key)
	if err != nil || !ok {
		return false, err
	}
	flow := &FlowState{ints: snapshot.Ints, bits: snapshot.Bits, maxNameBytes: s.cfg.MaxNameLength}
	return flow.CompareInt(name, op, rhs)
}
