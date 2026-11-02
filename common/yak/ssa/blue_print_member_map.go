package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// blueprintMemberMap publishes immutable member histories. The reverse lookup
// indexes field names, not Values, so it neither owns another IR graph nor
// changes assignment order. Assigned instruction IDs are stable; histories
// containing not-yet-emitted values are checked until their IDs are assigned.
// Every publication and lookup update holds the same lock.
type blueprintMemberMap struct {
	mu      sync.RWMutex
	fields  map[string][]Value
	byID    map[int64][]string
	pending map[string]struct{}
}

func newBlueprintMemberMap() *blueprintMemberMap {
	return &blueprintMemberMap{fields: make(map[string][]Value)}
}

func (m *blueprintMemberMap) Get(name string) ([]Value, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.fields[name]
	return values, ok
}

func (m *blueprintMemberMap) Have(name string) bool {
	if utils.IsNil(m) {
		return false
	}
	_, ok := m.Get(name)
	return ok
}

func (m *blueprintMemberMap) Keys() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.fields))
	for name := range m.fields {
		keys = append(keys, name)
	}
	return keys
}

// ForEach invokes callbacks outside the lock (inheritance can re-enter maps).
func (m *blueprintMemberMap) ForEach(fn func(string, []Value) bool) {
	if m == nil || fn == nil {
		return
	}
	m.mu.RLock()
	names := make([]string, 0, len(m.fields))
	values := make([][]Value, 0, len(m.fields))
	for name, history := range m.fields {
		names = append(names, name)
		values = append(values, history)
	}
	m.mu.RUnlock()
	for i, name := range names {
		if !fn(name, values[i]) {
			break
		}
	}
}

func (m *blueprintMemberMap) addReference(name string, id int64) {
	if id <= 0 {
		if m.pending == nil {
			m.pending = make(map[string]struct{})
		}
		m.pending[name] = struct{}{}
		return
	}
	if m.byID == nil {
		m.byID = make(map[int64][]string)
	}
	for _, existing := range m.byID[id] {
		if existing == name {
			return
		}
	}
	m.byID[id] = append(m.byID[id], name)
}

func (m *blueprintMemberMap) removeReference(name string, id int64) {
	names := m.byID[id]
	for i, existing := range names {
		if existing != name {
			continue
		}
		names[i] = names[len(names)-1]
		names[len(names)-1] = ""
		names = names[:len(names)-1]
		if len(names) == 0 {
			delete(m.byID, id)
		} else {
			m.byID[id] = names
		}
		return
	}
}

func (m *blueprintMemberMap) Set(name string, values []Value) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, value := range m.fields[name] {
		if !utils.IsNil(value) {
			m.removeReference(name, value.GetId())
		}
	}
	delete(m.pending, name)
	m.fields[name] = append([]Value(nil), values...)
	for _, value := range values {
		if !utils.IsNil(value) {
			m.addReference(name, value.GetId())
		}
	}
}

func (m *blueprintMemberMap) append(name string, value Value) bool {
	if m == nil || utils.IsNil(value) {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.fields[name]
	if len(old) > 0 && sameBlueprintMemberValue(old[len(old)-1], value) {
		return false
	}
	next := make([]Value, len(old)+1)
	copy(next, old)
	next[len(old)] = value
	m.fields[name] = next
	m.addReference(name, value.GetId())
	return true
}

func (m *blueprintMemberMap) replace(from, to Value) {
	if m == nil || utils.IsNil(from) {
		return
	}
	fromID := from.GetId()
	m.mu.Lock()
	defer m.mu.Unlock()

	for name := range m.pending {
		unassigned := false
		for _, value := range m.fields[name] {
			if utils.IsNil(value) {
				continue
			}
			id := value.GetId()
			if id <= 0 {
				unassigned = true
			} else {
				m.addReference(name, id)
			}
		}
		if !unassigned {
			delete(m.pending, name)
		}
	}

	names := m.byID[fromID]
	delete(m.byID, fromID)
	if fromID <= 0 {
		for name := range m.pending {
			names = append(names, name)
		}
	}
	for _, name := range names {
		old := m.fields[name]
		var next []Value
		for i, value := range old {
			if utils.IsNil(value) || value.GetId() != fromID {
				continue
			}
			if next == nil {
				next = append([]Value(nil), old...)
			}
			next[i] = to
		}
		if next == nil {
			continue
		}
		m.fields[name] = next
		if !utils.IsNil(to) {
			m.addReference(name, to.GetId())
		}
	}
}
