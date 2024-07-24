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

func (sm *SafeMap) ForEach(h func(m *SafeMap, key string, value any) error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	for k, v := range sm.m {
		if err := h(sm, k, v); err != nil {
			return
		}
	}
}

func (sm *SafeMap) Raw() map[string]any {
	return sm.m
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
	return sm.parent.Existed(p)
}

func (sm *SafeMap) SetPred(p *SafeMap) *SafeMap {
	if sm.Existed(p) {
		return sm
	}
	root := sm.GetRoot()
	root.parent = p
	return sm
}
