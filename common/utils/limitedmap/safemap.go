package limitedmap

import (
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type SafeMap struct {
	parent *SafeMap
	m      map[string]any
	lock   *sync.RWMutex
}

func NewSafeMap(a map[string]any) *SafeMap {
	if utils.IsNil(a) {
		a = make(map[string]any)
	}
	return &SafeMap{
		m:    a,
		lock: new(sync.RWMutex),
	}
}

func (sm *SafeMap) Append(l map[string]any) *SafeMap {
	if utils.IsNil(l) {
		return sm
	}
	sm.lock.RLock()
	defer sm.lock.RUnlock()
	return &SafeMap{
		parent: sm,
		m:      l,
		lock:   new(sync.RWMutex),
	}
}

func (sm *SafeMap) Load(key string) (value any, ok bool) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()
	value, ok = sm.m[key]
	if ok {
		if raw, ok := value.(map[string]any); ok && sm.parent != nil {
			if ext, ok2 := sm.parent.Load(key); ok2 {
				if extMap, typeOk := ext.(map[string]any); typeOk {
					for k, v := range extMap {
						_, existed := raw[k]
						if !existed {
							raw[k] = v
						}
					}
					return raw, true
				}
			}
		}
		return
	}

	if sm.parent != nil {
		return sm.parent.Load(key)
	}
	return
}

func (sm *SafeMap) ForEachKey(h func(m *SafeMap, key string) error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	visited := make(map[string]struct{})
	wrapper := func(safeMap *SafeMap, key string, value any) error {
		_, ok := visited[key]
		if ok {
			return nil
		}
		visited[key] = struct{}{}

		err := h(safeMap, key)
		if err != nil {
			return err
		}
		return nil
	}
	sm.forEachKey(wrapper)
}

func (sm *SafeMap) forEachKey(wrapper func(m *SafeMap, key string, value any) error) {
	for k, v := range sm.m {
		if err := wrapper(sm, k, v); err != nil {
			return
		}
	}
	if sm.parent != nil {
		sm.parent.forEachKey(wrapper)
	}
}

func (sm *SafeMap) Flat() map[string]any {
	var item []string
	sm.ForEachKey(func(m *SafeMap, key string) error {
		item = append(item, key)
		return nil
	})
	var m = make(map[string]any)
	for _, k := range item {
		raw, ok := sm.Load(k)
		if ok {
			m[k] = raw
		}
	}
	return m
}

func (sm *SafeMap) GetRoot() *SafeMap {
	if sm.parent == nil {
		return sm
	}
	return sm.parent.GetRoot()
}

func (sm *SafeMap) Existed(p *SafeMap) bool {
	if p == nil {
		return false
	}
	if sm == nil {
		return false
	}
	if sm == p {
		return true
	}
	for sm.parent != nil {
		if sm.parent == p {
			return true
		}
		sm = sm.parent
	}
	return false
}

func (sm *SafeMap) SetPred(p *SafeMap) *SafeMap {
	if sm.Existed(p) {
		return sm
	}
	root := sm.GetRoot()
	root.parent = p
	return sm
}

func (sm *SafeMap) Unlink(p *SafeMap) {
	if sm.Existed(p) {
		origin := sm.parent
		for origin.parent != nil {
			if origin.parent == p {
				origin.parent = nil
				return
			}
			origin = origin.parent
		}
	}
}

func (sm *SafeMap) Store(key string, value any) {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	sm.m[key] = value
}
