package limitedmap

import (
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type ReadOnlyMap struct {
	parent *ReadOnlyMap
	m      map[string]any
	lock   *sync.RWMutex
}

func NewReadOnlyMap(a map[string]any) *ReadOnlyMap {
	if utils.IsNil(a) {
		a = make(map[string]any)
	}
	return &ReadOnlyMap{
		m:    a,
		lock: new(sync.RWMutex),
	}
}

func (sm *ReadOnlyMap) Append(l map[string]any) *ReadOnlyMap {
	if utils.IsNil(l) {
		return sm
	}
	sm.lock.Lock()
	defer sm.lock.Unlock()

	if sm.m == nil || len(sm.m) <= 0 {
		return &ReadOnlyMap{
			m:    l,
			lock: new(sync.RWMutex),
		}
	}
	return &ReadOnlyMap{
		parent: sm,
		m:      l,
		lock:   new(sync.RWMutex),
	}
}

func (sm *ReadOnlyMap) Load(key string) (value any, ok bool) {
	sm.lock.Lock()
	defer sm.lock.Unlock()
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

func (sm *ReadOnlyMap) ForEachKey(h func(m any, key string) error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	visited := make(map[string]struct{})
	wrapper := func(safeMap any, key string, value any) error {
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

func (sm *ReadOnlyMap) forEachKey(wrapper func(m any, key string, value any) error) {
	for k, v := range sm.m {
		if err := wrapper(sm, k, v); err != nil {
			return
		}
	}
	if sm.parent != nil {
		sm.parent.forEachKey(wrapper)
	}
}

func (sm *ReadOnlyMap) Flat() map[string]any {
	var item []string
	sm.ForEachKey(func(m any, key string) error {
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

func (sm *ReadOnlyMap) GetRoot() *ReadOnlyMap {
	if sm.parent == nil {
		return sm
	}
	return sm.parent.GetRoot()
}

func (sm *ReadOnlyMap) Existed(p *ReadOnlyMap) bool {
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

func (sm *ReadOnlyMap) SetPred(p *ReadOnlyMap) *ReadOnlyMap {
	if sm.Existed(p) {
		return sm
	}
	root := sm.GetRoot()
	root.parent = p
	return sm
}

func (sm *ReadOnlyMap) Unlink(p *ReadOnlyMap) {
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

func (sm *ReadOnlyMap) Store(key string, value any) {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	sm.m[key] = value
}
