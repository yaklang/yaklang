package base

import "sync"

// configStore is a private, ordered key/value store for small parser configs.
// It preserves shallow values, insertion order and snapshot iteration. Small
// stores keep entries together; larger contexts acquire an index for lookup.
// All stores remain independent; no parsed state or mutable values are pooled.
type configStore struct {
	mu      sync.RWMutex
	entries []configEntry
	inline  [8]configEntry
	index   map[string]int
}

type configEntry struct {
	key   string
	value any
}

// configReplay is private until the function-valued journal is requested.
// Ordinary assignments need data, not a new closure and boxed slice per write.
// Once exposed, the store permanently uses the original []NodeConfigFun path
// until that journal is explicitly replaced or deleted. This preserves callers'
// mutable slice aliases, including custom callbacks and append capacity.
type configReplay struct {
	writes []configEntry
}

func configWriteOption(key string, value any) NodeConfigFun {
	return func(target *Config) { target.SetItem(key, value) }
}

func (s *configStore) exposeReplayLocked() {
	i, ok := s.findLocked(CfgOptionFuns)
	if !ok {
		return
	}
	journal, ok := s.entries[i].value.(*configReplay)
	if !ok {
		return
	}
	var options []NodeConfigFun
	// Append individually just as the original writer did. Besides order,
	// retain the runtime's slice capacity and resulting shallow-copy behavior.
	for _, write := range journal.writes {
		options = append(options, configWriteOption(write.key, write.value))
	}
	s.entries[i].value = options
}

func (s *configStore) findLocked(key string) (int, bool) {
	if s.index != nil {
		i, ok := s.index[key]
		return i, ok
	}
	for i := range s.entries {
		if s.entries[i].key == key {
			return i, true
		}
	}
	return 0, false
}

func (s *configStore) Get(key string) (any, bool) {
	if s == nil {
		return nil, false
	}
	if key == CfgOptionFuns {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.exposeReplayLocked()
		if i, ok := s.findLocked(key); ok {
			return s.entries[i].value, true
		}
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i, ok := s.findLocked(key); ok {
		return s.entries[i].value, true
	}
	return nil, false
}

func (s *configStore) Has(key string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.findLocked(key)
	return ok
}

func (s *configStore) Set(key string, value any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setLocked(key, value)
}

// setLocked is used only while the owning store is exclusively locked.
func (s *configStore) setLocked(key string, value any) {
	if i, ok := s.findLocked(key); ok {
		s.entries[i].value = value
		return
	}
	if s.entries == nil {
		s.entries = s.inline[:0]
	}
	spillInline := len(s.entries) == len(s.inline) && cap(s.entries) == len(s.inline)
	s.entries = append(s.entries, configEntry{key, value})
	if spillInline {
		clear(s.inline[:]) // do not retain stale values after moving to a larger slice
	}
	if s.index != nil {
		s.index[key] = len(s.entries) - 1
	} else if len(s.entries) > 16 {
		s.index = make(map[string]int, len(s.entries))
		for i, entry := range s.entries {
			s.index[entry.key] = i
		}
	}
}

// setConfigItem records the value and its replay operation under one lock.
// Do not invoke replay callbacks here: callbacks may reenter the store.
func (s *configStore) setConfigItem(key string, value any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setConfigItemLocked(key, value, 4)
}

func (s *configStore) setConfigItemLocked(key string, value any, reserve int) {
	s.setLocked(key, value)
	if i, ok := s.findLocked(CfgOptionFuns); ok {
		if journal, ok := s.entries[i].value.(*configReplay); ok {
			journal.writes = append(journal.writes, configEntry{key, value})
			return
		}
		// Keep the historical type assertion and write-before-panic behavior
		// for callers that explicitly install an invalid replay value.
		options := append(s.entries[i].value.([]NodeConfigFun), configWriteOption(key, value))
		s.entries[i].value = options
	} else {
		journal := &configReplay{writes: make([]configEntry, 1, reserve)}
		journal.writes[0] = configEntry{key, value}
		s.setLocked(CfgOptionFuns, journal)
	}
}

func (s *configStore) setConfigItems(items []ConfigItem) {
	if s == nil || len(items) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.setConfigItemLocked(item.Key, item.Value, len(items))
	}
}

func (s *configStore) inheritedItems() (items [3]configEntry, count int) {
	if s == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, key := range [...]string{CfgEndian, "parser", "unit"} {
		if i, ok := s.findLocked(key); ok {
			items[count] = s.entries[i]
			count++
		}
	}
	return
}

func (s *configStore) replayHistoryLen() (count int, present, wellFormed bool) {
	if s == nil {
		return 0, false, true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.findLocked(CfgOptionFuns)
	if !ok {
		return 0, false, true
	}
	switch journal := s.entries[i].value.(type) {
	case *configReplay:
		return len(journal.writes), true, true
	case []NodeConfigFun:
		return len(journal), true, true
	default:
		return 0, true, false
	}
}

func (s *configStore) replay(target *Config) {
	if s == nil {
		return
	}
	var writes []configEntry
	var options any
	var present bool
	// Snapshot ordinary writes. Custom functions keep their original slice
	// aliases and are invoked without the source lock, allowing reentrant edits.
	func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if i, ok := s.findLocked(CfgOptionFuns); ok {
			if journal, ok := s.entries[i].value.(*configReplay); ok {
				writes = append([]configEntry(nil), journal.writes...)
			} else {
				options, present = s.entries[i].value, true
			}
		}
	}()
	for _, write := range writes {
		target.SetItem(write.key, write.value)
	}
	if present {
		for _, option := range options.([]NodeConfigFun) {
			option(target)
		}
	}
}

func (s *configStore) Delete(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.findLocked(key)
	if !ok {
		return
	}
	copy(s.entries[i:], s.entries[i+1:])
	s.entries[len(s.entries)-1] = configEntry{}
	s.entries = s.entries[:len(s.entries)-1]
	if s.index != nil {
		delete(s.index, key)
		for j := i; j < len(s.entries); j++ {
			s.index[s.entries[j].key] = j
		}
	}
}

func (s *configStore) ForEach(handler func(string, any) bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.exposeReplayLocked()
	entries := append([]configEntry(nil), s.entries...)
	s.mu.Unlock()
	for _, entry := range entries {
		if !handler(entry.key, entry.value) {
			break
		}
	}
}
