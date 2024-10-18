package limitedmap

import (
	"sync"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
)

type SafeMap struct {
	parent *ReadOnlyMap
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

func (sm *SafeMap) Clone() *SafeMap {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	nsm := NewSafeMap(maps.Clone(sm.m))
	nsm.parent = sm.parent
	return nsm
}

// Deep1Clone deep clone the first level of the map
func (sm *SafeMap) Deep1Clone() *SafeMap {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	newMap := make(map[string]any, len(sm.m))

	for k, v := range sm.m {
		if vMap, ok := v.(map[string]any); ok {
			newMap[k] = maps.Clone(vMap)
		} else {
			newMap[k] = v
		}
	}

	nsm := NewSafeMap(newMap)
	nsm.parent = sm.parent
	return nsm
}

func (sm *SafeMap) Append(l map[string]any) *SafeMap {
	if utils.IsNil(l) {
		return sm
	}
	sm.lock.Lock()
	defer sm.lock.Unlock()

	for k, v := range l {
		origin, existed := sm.m[k]
		if !existed {
			sm.m[k] = v
			continue
		}
		if isMapKeyString(v) && isMapKeyString(origin) {
			funk.ForEach(v, func(key, value any) {
				keyStr := key.(string)
				setMapKeyValue(origin, keyStr, value)
			})
			continue
		}
		sm.m[k] = v
	}
	return sm
}

func (sm *SafeMap) Load(key string) (value any, ok bool) {
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

func (sm *SafeMap) ForEachKey(h func(m any, key string) error) {
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

func (sm *SafeMap) forEachKey(wrapper func(m any, key string, value any) error) {
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
	sm.ForEachKey(func(m any, key string) error {
		item = append(item, key)
		return nil
	})
	m := make(map[string]any)
	for _, k := range item {
		raw, ok := sm.Load(k)
		if ok {
			m[k] = raw
		}
	}
	return m
}

func (sm *SafeMap) GetRoot() *ReadOnlyMap {
	if sm.parent == nil {
		return nil
	}
	return sm.parent.GetRoot()
}

func (sm *SafeMap) Existed(p *ReadOnlyMap) bool {
	if p == nil {
		return false
	}
	if sm == nil {
		return false
	}
	if sm.parent == nil {
		return false
	}
	return sm.parent.Existed(p)
}

func (sm *SafeMap) SetPred(p *ReadOnlyMap) *SafeMap {
	sm.parent = p
	return sm
}

func (sm *SafeMap) Unlink() {
	sm.parent = nil
}

func (sm *SafeMap) Store(key string, value any) {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	sm.m[key] = value
}
