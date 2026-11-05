package state

import (
	"hash/fnv"
	"time"
)

// XBitScope selects the owner encoded by XBitKey.
type XBitScope uint8

const (
	XBitFlow XBitScope = iota + 1
	XBitHost
	XBitPair
)

// XBitKey is comparable. Construct it with one of the scope-specific helpers.
type XBitKey struct {
	Scope XBitScope
	Name  string
	Flow  FlowKey
	Host  HostKey
	Pair  PairKey
}

func FlowXBitKey(flow FlowKey, name string) XBitKey {
	return XBitKey{Scope: XBitFlow, Name: name, Flow: flow}
}

func HostXBitKey(host HostKey, name string) XBitKey {
	return XBitKey{Scope: XBitHost, Name: name, Host: host}
}

func PairXBitKey(pair PairKey, name string) XBitKey {
	return XBitKey{Scope: XBitPair, Name: name, Pair: pair}
}

type xbitEntry struct {
	expiresAt time.Time
	lastTime  time.Time
}

// SetXBit sets a scoped bit. ttl == 0 means no expiry; negative ttl is invalid.
func (s *Store) SetXBit(key XBitKey, ttl time.Duration) error {
	if ttl < 0 {
		return ErrInvalidArgument
	}
	if err := s.validateXBitKey(key); err != nil {
		return err
	}
	if err := s.begin(); err != nil {
		return err
	}
	defer s.end()
	sh := &s.xbits[hashXBitKey(key)%uint64(len(s.xbits))]
	sh.Lock()
	defer sh.Unlock()
	now := s.clock.Now()
	entry, exists := sh.items[key]
	if exists && now.Before(entry.lastTime) {
		now = entry.lastTime
	}
	if !exists && !s.reserve(&s.xbitCount, s.cfg.MaxXBits) {
		return ErrCapacity
	}
	entry.lastTime = now
	entry.expiresAt = time.Time{}
	if ttl > 0 {
		entry.expiresAt = now.Add(ttl)
	}
	sh.items[key] = entry
	return nil
}

func (s *Store) UnsetXBit(key XBitKey) (bool, error) {
	if err := s.validateXBitKey(key); err != nil {
		return false, err
	}
	if err := s.begin(); err != nil {
		return false, err
	}
	defer s.end()
	sh := &s.xbits[hashXBitKey(key)%uint64(len(s.xbits))]
	sh.Lock()
	defer sh.Unlock()
	if _, exists := sh.items[key]; !exists {
		return false, nil
	}
	delete(sh.items, key)
	s.xbitCount.Add(-1)
	return true, nil
}

func (s *Store) ToggleXBit(key XBitKey, ttl time.Duration) (bool, error) {
	if ttl < 0 {
		return false, ErrInvalidArgument
	}
	if err := s.validateXBitKey(key); err != nil {
		return false, err
	}
	if err := s.begin(); err != nil {
		return false, err
	}
	defer s.end()
	sh := &s.xbits[hashXBitKey(key)%uint64(len(s.xbits))]
	sh.Lock()
	defer sh.Unlock()
	now := s.clock.Now()
	if entry, exists := sh.items[key]; exists {
		if now.Before(entry.lastTime) {
			now = entry.lastTime
		}
		if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			delete(sh.items, key)
			s.xbitCount.Add(-1)
			return false, nil
		}
		delete(sh.items, key)
		s.xbitCount.Add(-1)
	}
	if !s.reserve(&s.xbitCount, s.cfg.MaxXBits) {
		return false, ErrCapacity
	}
	entry := xbitEntry{lastTime: now}
	if ttl > 0 {
		entry.expiresAt = now.Add(ttl)
	}
	sh.items[key] = entry
	return true, nil
}

func (s *Store) TestXBit(key XBitKey) (bool, error) {
	if err := s.validateXBitKey(key); err != nil {
		return false, err
	}
	if err := s.begin(); err != nil {
		return false, err
	}
	defer s.end()
	sh := &s.xbits[hashXBitKey(key)%uint64(len(s.xbits))]
	sh.Lock()
	defer sh.Unlock()
	entry, exists := sh.items[key]
	if !exists {
		return false, nil
	}
	now := s.clock.Now()
	if now.Before(entry.lastTime) {
		now = entry.lastTime
	}
	if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		delete(sh.items, key)
		s.xbitCount.Add(-1)
		return false, nil
	}
	entry.lastTime = now
	sh.items[key] = entry
	return true, nil
}

func (s *Store) validateXBitKey(key XBitKey) error {
	if err := s.validName(key.Name); err != nil {
		return err
	}
	switch key.Scope {
	case XBitFlow:
		if !validFlowKey(key.Flow) || key.Host != (HostKey{}) || key.Pair != (PairKey{}) {
			return ErrInvalidKey
		}
	case XBitHost:
		if !validHostKey(key.Host) || key.Flow != (FlowKey{}) || key.Pair != (PairKey{}) {
			return ErrInvalidKey
		}
	case XBitPair:
		if !validPairKey(key.Pair) || key.Flow != (FlowKey{}) || key.Host != (HostKey{}) {
			return ErrInvalidKey
		}
	default:
		return ErrInvalidKey
	}
	return nil
}

func hashXBitKey(key XBitKey) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte{byte(key.Scope)})
	_, _ = h.Write([]byte(key.Name))
	switch key.Scope {
	case XBitFlow:
		writeUint64(h, hashFlowKey(key.Flow))
	case XBitHost:
		writeUint64(h, key.Host.Epoch)
		writeDomain(h, key.Host.Domain)
		writeAddr(h, key.Host.Addr)
	case XBitPair:
		writeUint64(h, key.Pair.Epoch)
		writeDomain(h, key.Pair.Domain)
		writeAddr(h, key.Pair.A)
		writeAddr(h, key.Pair.B)
	}
	return h.Sum64()
}
